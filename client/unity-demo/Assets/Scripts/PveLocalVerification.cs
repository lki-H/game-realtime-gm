using System;
using System.IO;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEngine;

public sealed class PveLocalVerification : MonoBehaviour
{
    [RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.AfterSceneLoad)]
    private static void Bootstrap()
    {
        if (Environment.GetEnvironmentVariable("PVE_DEMO_AUTOMATION") != "1") return;
        var root = new GameObject("Local PVE verification");
        root.AddComponent<PveControlClient>();
        root.AddComponent<PveLocalVerification>();
    }

    private async void Start()
    {
        var client = GetComponent<PveControlClient>();
        var output = Environment.GetEnvironmentVariable("PVE_DEMO_RESULT");
        var report = new JObject { ["success"] = false, ["stage"] = "login" };
        try
        {
            var endpoint = Environment.GetEnvironmentVariable("PVE_DEMO_SERVER");
            if (!Uri.TryCreate(endpoint, UriKind.Absolute, out var address) || address.Scheme != "http" || !address.IsLoopback) throw new InvalidOperationException("Local HTTP server required");
            if (string.IsNullOrEmpty(output)) throw new InvalidOperationException("Result path required");
            client.ServerUrl = endpoint;
            var username = Environment.GetEnvironmentVariable("PVE_DEMO_USERNAME");
            var password = Environment.GetEnvironmentVariable("PVE_DEMO_PASSWORD");
            Environment.SetEnvironmentVariable("PVE_DEMO_PASSWORD", null);
            await client.Login(username, password);
            password = null;
            report["player_id"] = client.PlayerId;
            report["stage"] = "queued";
            var taskKey = Environment.GetEnvironmentVariable("PVE_DEMO_TASK") ?? "";
            string partyId = null;
            var partyRole = Environment.GetEnvironmentVariable("PVE_DEMO_PARTY_ROLE");
            if (partyRole == "owner")
            {
                var party = (await client.Request("v2.party.create", new { }))["data"];
                partyId = (string)party["id"];
                var peer = long.Parse(Environment.GetEnvironmentVariable("PVE_DEMO_PEER"));
                var invitation = (await client.Request("v2.party.invite", new { party_id = partyId, player_id = peer }))["data"];
                var invitePath = Environment.GetEnvironmentVariable("PVE_DEMO_INVITE_FILE");
                File.WriteAllText(invitePath, ((string)invitation["token"]));
                var rosterDeadline = DateTime.UtcNow.AddSeconds(15);
                while (DateTime.UtcNow < rosterDeadline)
                {
                    party = (await client.Query("/api/v2/parties/" + partyId))["data"];
                    if ((party["members"] as JArray)?.Count == 2) break;
                    await Task.Delay(100);
                }
                if ((party["members"] as JArray)?.Count != 2) throw new TimeoutException("Party join timeout");
                await client.Request("v2.party.plan_update", new { party_id = partyId, plan = new { operation = "training_ground", difficulty = "normal", rule_version = "training_ground.v1", fill_policy = "public", allow_partial = false } });
                await client.Request("v2.party.selection", new { party_id = partyId, task_key = taskKey, task_version = "training_ground.v1" });
                File.WriteAllText(invitePath + ".ready", partyId);
                while (!File.Exists(invitePath + ".selected") && DateTime.UtcNow < rosterDeadline) await Task.Delay(100);
                if (!File.Exists(invitePath + ".selected")) throw new TimeoutException("Member selection timeout");
            }
            else if (partyRole == "member")
            {
                var invitePath = Environment.GetEnvironmentVariable("PVE_DEMO_INVITE_FILE");
                var inviteDeadline = DateTime.UtcNow.AddSeconds(15);
                while (!File.Exists(invitePath) && DateTime.UtcNow < inviteDeadline) await Task.Delay(100);
                var token = File.ReadAllText(invitePath);
                var party = (await client.Request("v2.party.accept_invite", new { token }))["data"];
                File.Delete(invitePath);
                partyId = (string)party["id"];
                while (!File.Exists(invitePath + ".ready") && DateTime.UtcNow < inviteDeadline) await Task.Delay(100);
                if (!File.Exists(invitePath + ".ready")) throw new TimeoutException("Party setup timeout");
                await client.Request("v2.party.selection", new { party_id = partyId, task_key = taskKey, task_version = "training_ground.v1" });
                File.WriteAllText(invitePath + ".selected", partyId);
            }
            if (partyId != null)
            {
                await Task.Delay(300);
                var party = (await client.Query("/api/v2/parties/" + partyId))["data"];
                JToken self = null;
                foreach (var member in party["members"]) if ((long)member["player_id"] == client.PlayerId) self = member;
                await client.Request("v2.party.ready", new { party_id = partyId, ready = true, roster_version = (long)party["roster_version"], plan_version = (long)party["plan_version"], selection_version = (long)self["selection_version"] });
                if (partyRole == "owner")
                {
                    var readyDeadline = DateTime.UtcNow.AddSeconds(10);
                    var allReady = false;
                    while (DateTime.UtcNow < readyDeadline)
                    {
                        party = (await client.Query("/api/v2/parties/" + partyId))["data"];
                        allReady = true;
                        foreach (var member in party["members"]) if ((bool?)member["ready"] != true) allReady = false;
                        if (allReady) break;
                        await Task.Delay(100);
                    }
                    if (!allReady) throw new TimeoutException("Party ready timeout");
                    await client.Request("v2.match.enqueue", new { party_id = partyId });
                }
                report["party_id"] = partyId;
            }
            else await client.Request("v2.match.enqueue", new
            {
                plan = new { operation = "training_ground", difficulty = "normal", rule_version = "training_ground.v1", fill_policy = "public", allow_partial = false },
                task = new { task_key = taskKey, task_version = "training_ground.v1" }
            });
            var deadline = DateTime.UtcNow.AddMinutes(3);
            string runId = null;
            string confirmedProposal = null;
            while (DateTime.UtcNow < deadline)
            {
                var activity = (await client.Query("/api/v2/me/activity"))["data"] as JObject;
                var proposal = activity?["proposal"] as JObject;
                if (proposal != null && (string)proposal["id"] != confirmedProposal)
                {
                    confirmedProposal = (string)proposal["id"];
                    await client.Request("v2.match.proposal_confirm", new { proposal_id = confirmedProposal, revision = (long)proposal["revision"] });
                }
                runId = (string)activity?["run"]?["id"];
                if (!string.IsNullOrEmpty(runId)) break;
                await Task.Delay(250);
            }
            if (string.IsNullOrEmpty(runId)) throw new TimeoutException("Assignment timeout");
            report["run_id"] = runId;
            var before = await client.Snapshot(runId);
            var attemptBefore = PersonalAttempt(before, client.PlayerId);
            await client.Disconnect();
            await Task.Delay(300);
            await client.Connect();
            var after = await client.Snapshot(runId);
            if ((string)after["data"]?["id"] != runId || PersonalAttempt(after, client.PlayerId) != attemptBefore) throw new InvalidOperationException("Reconnect identity changed");
            report["reconnected"] = true;
            report["stage"] = "assigned";
            Write(output, report);
            while (DateTime.UtcNow < deadline)
            {
                var result = await client.Query("/api/v2/runs/" + Uri.EscapeDataString(runId) + "/results");
                if ((bool?)result["data"]?["settled"] == true)
                {
                    report["success"] = true;
                    report["stage"] = "settled";
                    report["task_attempt_id"] = attemptBefore;
                    report["result_status"] = result["data"]?["participant_result"]?["result_status"];
                    Write(output, report);
                    await client.Disconnect();
                    Application.Quit(0);
                    return;
                }
                await Task.Delay(500);
            }
            throw new TimeoutException("Settlement timeout");
        }
        catch (Exception failure)
        {
            report["failure_type"] = failure.GetType().Name;
            report["failure"] = failure is InvalidOperationException ? failure.Message : "verification did not complete";
            if (!string.IsNullOrEmpty(output)) Write(output, report);
            await client.Disconnect();
            Application.Quit(1);
        }
    }

    private static string PersonalAttempt(JObject snapshot, long playerId)
    {
        foreach (var participant in snapshot["data"]?["participants"] as JArray ?? new JArray())
            if ((long?)participant["player_id"] == playerId) return (string)participant["task_attempt_id"];
        throw new InvalidOperationException("Personal participant missing");
    }

    private static void Write(string path, JObject value)
    {
        File.WriteAllText(path + ".tmp", value.ToString());
        if (File.Exists(path)) File.Replace(path + ".tmp", path, null);
        else File.Move(path + ".tmp", path);
    }
}
