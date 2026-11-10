using Google.Protobuf;
using Pve.Reporting.V1;
using System.Text.Json;

var fixturePath = args.Length == 1 ? args[0] : "fixtures/protobuf-golden.json";
using var fixture = JsonDocument.Parse(File.ReadAllText(fixturePath));
var unknown = Convert.FromHexString(fixture.RootElement.GetProperty("unknown_hex").GetString()!);
var cases = fixture.RootElement.GetProperty("cases");
if (cases.GetArrayLength() != 4 || unknown.Length == 0) throw new InvalidOperationException("Incomplete shared golden fixture");
foreach (var item in cases.EnumerateArray())
{
    var type = item.GetProperty("type").GetString()!;
    var wire = Convert.FromHexString(item.GetProperty("hex").GetString()!);
    var expected = Expected(type, item.GetProperty("expected").GetRawText());
    var restored = Parse(type, wire);
    if (!restored.Equals(expected) || !expected.ToByteArray().SequenceEqual(wire))
        throw new InvalidOperationException($"Shared golden fields or C# encoding changed: {type}");
    var future = wire.Concat(unknown).ToArray();
    var withUnknown = Parse(type, future);
    if (!withUnknown.ToByteArray().SequenceEqual(future) || JsonFormatter.Default.Format(withUnknown) != JsonFormatter.Default.Format(expected))
        throw new InvalidOperationException($"Unknown-field compatibility changed: {type}");
}
Console.WriteLine("Reporting Go/C# shared golden fields, bytes and unknown-field roundtrip: PASS");

static IMessage Expected(string type, string json) => type switch
{
    "GetLeaderboardRequest" => JsonParser.Default.Parse<GetLeaderboardRequest>(json),
    "GetLeaderboardResponse" => JsonParser.Default.Parse<GetLeaderboardResponse>(json),
    "GetRunResultRequest" => JsonParser.Default.Parse<GetRunResultRequest>(json),
    "GetRunResultResponse" => JsonParser.Default.Parse<GetRunResultResponse>(json),
    _ => throw new InvalidOperationException("Unsupported golden type")
};
static IMessage Parse(string type, byte[] bytes) => type switch
{
    "GetLeaderboardRequest" => GetLeaderboardRequest.Parser.ParseFrom(bytes),
    "GetLeaderboardResponse" => GetLeaderboardResponse.Parser.ParseFrom(bytes),
    "GetRunResultRequest" => GetRunResultRequest.Parser.ParseFrom(bytes),
    "GetRunResultResponse" => GetRunResultResponse.Parser.ParseFrom(bytes),
    _ => throw new InvalidOperationException("Unsupported golden type")
};
