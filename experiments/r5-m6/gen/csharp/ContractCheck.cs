using Google.Protobuf;
using Pve.Reporting.V1;

var request = new GetRunResultRequest { RunId = "run_contract_fixture", PlayerId = 1001 };
var restored = GetRunResultRequest.Parser.ParseFrom(request.ToByteArray());
if (restored.RunId != request.RunId || restored.PlayerId != request.PlayerId)
{
    throw new InvalidOperationException("Reporting contract roundtrip failed");
}
Console.WriteLine("Reporting C# contract: PASS");
