using System;
using System.Collections.Generic;
using System.Net.Http;
using System.Net.WebSockets;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.IO;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using UnityEngine;

public sealed class PveControlClient : MonoBehaviour
{
    public string ServerUrl = "http://127.0.0.1:8080";
    public event Action<JObject> Received;
    public event Action<string> ConnectionStateChanged;
    private readonly HttpClient client = new HttpClient { Timeout = TimeSpan.FromSeconds(10), MaxResponseContentBufferSize = 1024 * 1024 };
    private readonly Queue<Action> callbacks = new Queue<Action>();
    private readonly Dictionary<string, TaskCompletionSource<JObject>> requests = new Dictionary<string, TaskCompletionSource<JObject>>();
    private readonly SemaphoreSlim connectLock = new SemaphoreSlim(1, 1);
    private CancellationTokenSource lifetime;
    private ClientWebSocket socket;
    private string token;
    private int generation;
    private int sessionGeneration;
    private bool destroyed;
    public long PlayerId { get; private set; }
    private readonly SemaphoreSlim writeLock = new SemaphoreSlim(1, 1);

    public bool IsConnected => !destroyed && socket != null && socket.State == WebSocketState.Open;

    private void Awake() { lifetime = new CancellationTokenSource(); }
    public async Task Login(string username, string password)
    {
        var currentSession = Interlocked.Increment(ref sessionGeneration);
        var body = new StringContent(JsonConvert.SerializeObject(new { username, password }), Encoding.UTF8, "application/json");
        using (body)
        using (var response = await client.PostAsync(ServerUrl + "/api/login", body, lifetime.Token))
        {
            var result = JObject.Parse(await response.Content.ReadAsStringAsync());
            if (destroyed || currentSession != sessionGeneration) throw new InvalidOperationException("Session changed; stale login discarded");
            if (!response.IsSuccessStatusCode || (int?)result["code"] != 0) throw new InvalidOperationException((string)result["message"] ?? "Login failed");
            await Disconnect();
            token = (string)result["data"]["token"];
            PlayerId = (long)result["data"]["player"]["id"];
        }
        await Connect();
    }
    public async Task Connect()
    {
        if (string.IsNullOrEmpty(token)) throw new InvalidOperationException("Login first");
        await connectLock.WaitAsync(lifetime.Token);
        try
        {
            var currentGeneration = Interlocked.Increment(ref generation);
            RejectPending();
            var connection = new ClientWebSocket();
            connection.Options.SetRequestHeader("Authorization", "Bearer " + token);
            var address = new Uri(ServerUrl.TrimEnd('/').Replace("https://", "wss://").Replace("http://", "ws://") + "/ws");
            var previous = socket;
            socket = connection;
            previous?.Abort(); previous?.Dispose();
            try
            {
                using var deadline = CancellationTokenSource.CreateLinkedTokenSource(lifetime.Token);
                deadline.CancelAfter(TimeSpan.FromSeconds(10));
                await connection.ConnectAsync(address, deadline.Token);
                if (!Current(connection, currentGeneration)) throw new InvalidOperationException("Connection replaced");
            }
            catch
            {
                if (Current(connection, currentGeneration)) socket = null;
                connection.Abort(); connection.Dispose();
                throw;
            }
            Enqueue(() => { if (Current(connection, currentGeneration)) ConnectionStateChanged?.Invoke("connected"); });
            _ = ReceiveLoop(connection, currentGeneration);
        }
        finally { connectLock.Release(); }
    }
    public Task Disconnect()
    {
        Interlocked.Increment(ref generation);
        var previous = socket;
        socket = null;
        RejectPending();
        previous?.Abort(); previous?.Dispose();
        Enqueue(() => ConnectionStateChanged?.Invoke("disconnected"));
        return Task.CompletedTask;
    }
    public async Task Send(string type, object data, string operationId = null)
    {
        await Request(type, data, operationId);
    }
    public async Task<JObject> Request(string type, object data, string operationId = null)
    {
        if (!IsConnected) throw new InvalidOperationException("Connect first");
        if (!type.StartsWith("v2.", StringComparison.Ordinal)) throw new ArgumentException("V2 command required");
        var requestId = Guid.NewGuid().ToString("N");
        var payload = JsonConvert.SerializeObject(new { type, schema_version = 2, request_id = requestId, operation_id = operationId ?? Guid.NewGuid().ToString("N"), data });
        if (Encoding.UTF8.GetByteCount(payload) > 16384) throw new ArgumentException("Command too large");
        var connection = socket;
        var currentGeneration = generation;
        var completion = new TaskCompletionSource<JObject>(TaskCreationOptions.RunContinuationsAsynchronously);
        lock (requests) { if (requests.Count >= 64) throw new InvalidOperationException("Too many pending commands"); requests.Add(requestId, completion); }
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(lifetime.Token);
        deadline.CancelAfter(TimeSpan.FromSeconds(10));
        using var registration = deadline.Token.Register(() => completion.TrySetCanceled());
        try
        {
            await writeLock.WaitAsync(deadline.Token);
            try
            {
                if (!Current(connection, currentGeneration)) throw new InvalidOperationException("Connection replaced; reconnect and recover current activity");
                await connection.SendAsync(new ArraySegment<byte>(Encoding.UTF8.GetBytes(payload)), WebSocketMessageType.Text, true, deadline.Token);
            }
            finally { writeLock.Release(); }
            var response = await completion.Task;
            if ((int?)response["code"] != 0) throw new InvalidOperationException(type+" rejected code="+(int?)response["code"]);
            return response;
        }
        finally { lock (requests) { requests.Remove(requestId); } }
    }
    public async Task<JObject> Snapshot(string runId)
    {
        return await Query("/api/v2/runs/" + Uri.EscapeDataString(runId) + "/snapshot");
    }
    public async Task<JObject> Query(string path)
    {
        if (!path.StartsWith("/api/v2/", StringComparison.Ordinal)) throw new ArgumentException("V2 query path required");
        var currentSession = sessionGeneration;
        var currentToken = token;
        using (var request = new HttpRequestMessage(HttpMethod.Get, ServerUrl.TrimEnd('/') + path))
        {
            request.Headers.Add("Authorization", "Bearer " + currentToken);
            using (var response = await client.SendAsync(request, lifetime.Token))
            {
                var result = JObject.Parse(await response.Content.ReadAsStringAsync());
                if (destroyed || currentSession != sessionGeneration || token != currentToken) throw new InvalidOperationException("Session changed; stale query discarded");
                if ((int)response.StatusCode == 401 || (int?)result["code"] == 40321) ExpireSession();
                if (!response.IsSuccessStatusCode || (int?)result["code"] != 0) throw new InvalidOperationException((string)result["message"] ?? "Request failed");
                return result;
            }
        }
    }
    private bool Current(ClientWebSocket connection, int currentGeneration)
    {
        return !destroyed && ReferenceEquals(socket, connection) && generation == currentGeneration;
    }
    private async Task ReceiveLoop(ClientWebSocket connection, int currentGeneration)
    {
        var buffer = new byte[4096];
        try
        {
            while (Current(connection, currentGeneration) && connection.State == WebSocketState.Open)
            {
                using var payload = new MemoryStream(); WebSocketReceiveResult received;
                do
                {
                    received = await connection.ReceiveAsync(new ArraySegment<byte>(buffer), lifetime.Token);
                    if (received.MessageType == WebSocketMessageType.Close) return;
                    if (received.MessageType != WebSocketMessageType.Text) throw new InvalidOperationException("Text response required");
                    payload.Write(buffer, 0, received.Count);
                    if (payload.Length > 1024 * 1024) throw new InvalidOperationException("Response too large");
                } while (!received.EndOfMessage);
                var message = JObject.Parse(Encoding.UTF8.GetString(payload.ToArray()));
                if (!Current(connection, currentGeneration)) return;
                var requestId = (string)message["request_id"];
                lock (requests) { if (requestId != null && requests.TryGetValue(requestId, out var completion)) completion.TrySetResult(message); }
                if ((int?)message["code"] == 40321) { ExpireSession(); return; }
                Enqueue(() => { if (Current(connection, currentGeneration)) Received?.Invoke(message); });
            }
        }
        catch (OperationCanceledException) { }
        catch (Exception failure) { Enqueue(() => { if (Current(connection, currentGeneration)) ConnectionStateChanged?.Invoke(failure.Message); }); }
        finally
        {
            if (Current(connection, currentGeneration))
            {
                connection.Abort();
                RejectPending();
                Enqueue(() => { if (Current(connection, currentGeneration)) ConnectionStateChanged?.Invoke("disconnected"); });
            }
        }
    }
    private void RejectPending()
    {
        lock (requests)
        {
            foreach (var completion in requests.Values) completion.TrySetException(new InvalidOperationException("Connection ended; reconnect and recover current activity before retrying the same operation"));
            requests.Clear();
        }
    }
    private void ExpireSession()
    {
        Interlocked.Increment(ref sessionGeneration);
        token = null;
        PlayerId = 0;
        _ = Disconnect();
        Enqueue(() => ConnectionStateChanged?.Invoke("Session expired; login again"));
    }
    private void Enqueue(Action action)
    {
        lock (callbacks)
        {
            if (callbacks.Count >= 256)
            {
                callbacks.Clear();
                callbacks.Enqueue(() => { _ = Disconnect(); ConnectionStateChanged?.Invoke("State queue exceeded capacity; reconnect and recover current activity"); });
                return;
            }
            callbacks.Enqueue(action);
        }
    }
    private void Update() { while (true) { Action action; lock (callbacks) { if (callbacks.Count == 0) return; action = callbacks.Dequeue(); } action(); } }
    private void OnDestroy() { destroyed = true; Interlocked.Increment(ref sessionGeneration); lifetime.Cancel(); RejectPending(); socket?.Abort(); socket?.Dispose(); client.Dispose(); }
}
