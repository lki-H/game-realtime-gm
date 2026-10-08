using System;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEngine;

public sealed class PveDemoPanel : MonoBehaviour
{
    private PveControlClient client;
    private string username = "";
    private string password = "";
    private string partyId = "";
    private string proposalId = "";
    private string runId = "";
    private string peer = "";
    private string inviteToken = "";
    private string taskKey = "";
	private string ruleVersion = "training_ground.v2";
	private string taskVersion = "training_ground.v1";
	private string regroupId = "";
	private string regroupPlayers = "";
	private string recruitmentPartyId = "";
    private long rosterVersion = 1;
    private long planVersion = 1;
    private long selectionVersion = 1;
    private long proposalRevision = 1;
    private long regroupRevision = 1;
    private string status = "登录后创建房间或单排。局内事实由受限事件 Bot 提供。";
    private bool busy;
    private Vector2 scroll;

    [RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.AfterSceneLoad)]
    private static void Bootstrap()
    {
        if (FindFirstObjectByType<PveDemoPanel>() != null) return;
        var root = new GameObject("PVE Control Demo");
        root.AddComponent<PveControlClient>();
        root.AddComponent<PveDemoPanel>();
    }
    private void Awake()
    {
        client = GetComponent<PveControlClient>();
        client.Received += Receive;
        client.ConnectionStateChanged += value => status = value;
    }
    private void Receive(JObject message)
    {
        status = message.ToString();
        var data = message["data"] as JObject;
        if (data == null) return;
        if (data["id"] != null && data["roster_version"] != null && data["members"] != null)
        {
            RestoreParty(data, true);
        }
        var kind = (string)message["type"] ?? "";
        if (data["proposal_id"] != null)
        {
            if (kind.StartsWith("v2.party.regroup", StringComparison.Ordinal)) { regroupId = (string)data["proposal_id"]; regroupRevision = (long?)data["revision"] ?? 1; }
            else { proposalId = (string)data["proposal_id"]; proposalRevision = (long?)data["revision"] ?? 1; }
        }
        if (kind == "v2.party.invited") inviteToken = (string)data["token"] ?? "";
        if (kind == "v2.recruitment.accepted") recruitmentPartyId = (string)data["party_id"] ?? "";
        if (data["run_id"] != null) runId = (string)data["run_id"];
    }
    private void RestoreSelection(JToken members)
    {
        if (members == null) return;
        foreach (var member in members)
        {
            if ((long?)member["player_id"] != client.PlayerId) continue;
            selectionVersion = (long?)member["selection_version"] ?? 1;
            taskKey = (string)member["task_selection"]?["task_key"] ?? "";
            taskVersion = (string)member["task_selection"]?["task_version"] ?? ruleVersion;
        }
    }
    private void RestoreParty(JObject party, bool membership)
    {
        if (party == null) return;
        if (membership) partyId = (string)party["id"] ?? "";
        rosterVersion = (long?)party["roster_version"] ?? 1;
        planVersion = (long?)party["plan_version"] ?? 1;
        ruleVersion = (string)party["plan"]?["rule_version"] ?? ruleVersion;
        if (membership) RestoreSelection(party["members"]);
    }
    private async Task RestoreRecruitment()
    {
        var response = await client.Query("/api/v2/recruitment/parties/" + Uri.EscapeDataString(recruitmentPartyId));
        var data = response["data"];
        RestoreParty(data?["party"] as JObject, false);
        RestoreSelection(data?["guests"]);
        status = response.ToString();
    }
    private async Task RestoreActivity()
    {
        var response = await client.Query("/api/v2/me/activity");
        var data = response["data"];
        partyId = (string)data?["party"]?["id"] ?? "";
        RestoreParty(data?["party"] as JObject, true);
        proposalId = (string)data?["proposal"]?["id"] ?? "";
        proposalRevision = (long?)data?["proposal"]?["revision"] ?? 1;
        runId = (string)data?["run"]?["id"] ?? (string)data?["latest_result"]?["run_id"] ?? "";
        var recruitment = data?["recruitment"] as JArray;
        recruitmentPartyId = recruitment != null && recruitment.Count > 0 ? (string)recruitment[0]["party_id"] : "";
        var regroup = data?["regroup_proposals"] as JArray;
        regroupId = regroup != null && regroup.Count > 0 ? (string)regroup[0]["id"] : "";
        if (!string.IsNullOrEmpty(recruitmentPartyId)) await RestoreRecruitment();
        status = response.ToString();
    }
    private async void Invoke(Func<Task> action)
    {
        if (busy) return;
        busy = true;
        try { await action(); }
        catch (Exception failure) { status = failure.Message; }
        finally { busy = false; }
    }
    private void OnGUI()
    {
        GUILayout.BeginArea(new Rect(24, 24, Mathf.Min(720, Screen.width - 48), Screen.height - 48), GUI.skin.box);
        scroll = GUILayout.BeginScrollView(scroll);
        GUILayout.Label("合作 PVE · V2 控制面验证");
        GUILayout.Label("服务地址"); client.ServerUrl = GUILayout.TextField(client.ServerUrl);
        GUILayout.Label("账号"); username = GUILayout.TextField(username);
        GUILayout.Label("密码"); password = GUILayout.PasswordField(password, '*');
        GUI.enabled = !busy;
        if (GUILayout.Button("登录并连接")) Invoke(async () => { await client.Login(username, password); password = ""; await RestoreActivity(); });
        if (GUILayout.Button("重连")) Invoke(async () => { await client.Connect(); await RestoreActivity(); });
        if (GUILayout.Button("创建好友房间")) Invoke(() => client.Send("v2.party.create", new { }));
        GUILayout.Label("房间 ID"); partyId = GUILayout.TextField(partyId);
        GUILayout.Label("目标玩家 ID"); peer = GUILayout.TextField(peer);
        if (GUILayout.Button("邀请玩家") && long.TryParse(peer, out var playerId)) Invoke(() => client.Send("v2.party.invite", new { party_id = partyId, player_id = playerId }));
        GUILayout.Label("接收的邀请凭证"); inviteToken = GUILayout.TextField(inviteToken);
        if (GUILayout.Button("接受邀请")) Invoke(() => client.Send("v2.party.accept_invite", new { token = inviteToken }));
        if (GUILayout.Button("开启公共补位")) Invoke(() => client.Send("v2.party.plan_update", new { party_id = partyId, plan = new { operation = "training_ground", difficulty = "normal", rule_version = ruleVersion, fill_policy = "public", allow_partial = true } }));
        GUILayout.Label("关卡规则版本"); ruleVersion = GUILayout.TextField(ruleVersion);
        GUILayout.Label("个人任务（空表示不携带）"); taskKey = GUILayout.TextField(taskKey);
        GUILayout.Label("个人任务逻辑版本（以目录返回为准）"); taskVersion = GUILayout.TextField(taskVersion);
        if (GUILayout.Button("查询本人任务目录")) Invoke(async () => status = (await client.Query("/api/v2/tasks?rule_version=" + Uri.EscapeDataString(ruleVersion))).ToString());
        if (GUILayout.Button("预览任务与关卡兼容")) Invoke(async () => status = (await client.Query("/api/v2/operations/preview?rule_version=" + Uri.EscapeDataString(ruleVersion) + "&task_key=" + Uri.EscapeDataString(taskKey))).ToString());
        if (GUILayout.Button("选择个人任务")) Invoke(() => client.Send("v2.party.selection", new { party_id = partyId, task_key = taskKey, task_version = taskVersion }));
        if (GUILayout.Button("停用并保留确认进度")) Invoke(() => client.Send("v2.task.pause", new { task_key = taskKey, task_version = taskVersion }));
        if (GUILayout.Button("准备")) Invoke(() => client.Send("v2.party.ready", new { party_id = partyId, ready = true, roster_version = rosterVersion, plan_version = planVersion, selection_version = selectionVersion }));
        if (GUILayout.Button("房间整组排队")) Invoke(() => client.Send("v2.match.enqueue", new { party_id = partyId }));
        if (GUILayout.Button("个人公共匹配")) Invoke(() => client.Send("v2.match.enqueue", new { plan = new { operation = "training_ground", difficulty = "normal", rule_version = ruleVersion, fill_policy = "public", allow_partial = true }, task = new { task_key = taskKey, task_version = taskVersion } }));
        GUILayout.Label("待确认候选 ID"); proposalId = GUILayout.TextField(proposalId);
        if (GUILayout.Button("接受候选")) Invoke(() => client.Send("v2.match.proposal_confirm", new { proposal_id = proposalId, revision = proposalRevision }));
        if (GUILayout.Button("拒绝候选")) Invoke(() => client.Send("v2.match.proposal_reject", new { proposal_id = proposalId, revision = proposalRevision }));
        GUILayout.Label("Run ID"); runId = GUILayout.TextField(runId);
        if (GUILayout.Button("查询当前进度 / 结算")) Invoke(async () => status = (await client.Snapshot(runId)).ToString());
		if (GUILayout.Button("统一恢复当前活动")) Invoke(RestoreActivity);
		GUILayout.Label("招募来源房间 ID"); recruitmentPartyId = GUILayout.TextField(recruitmentPartyId);
		if (GUILayout.Button("查询招募版本与准备状态")) Invoke(RestoreRecruitment);
		if (GUILayout.Button("招募成员选择任务")) Invoke(async () => { await client.Send("v2.recruitment.selection", new { party_id = recruitmentPartyId, task_key = taskKey, task_version = taskVersion }); await RestoreRecruitment(); });
		if (GUILayout.Button("招募成员准备")) Invoke(() => client.Send("v2.recruitment.ready", new { party_id = recruitmentPartyId, ready = true, roster_version = rosterVersion, plan_version = planVersion, selection_version = selectionVersion }));
		if (GUILayout.Button("退出招募")) Invoke(() => client.Send("v2.recruitment.leave", new { party_id = recruitmentPartyId }));
		GUILayout.Label("续组目标玩家 ID（逗号分隔）"); regroupPlayers = GUILayout.TextField(regroupPlayers);
		if (GUILayout.Button("发起自愿续组")) Invoke(() => client.Send("v2.party.regroup_propose", new { run_id = runId, owner_id = client.PlayerId, player_ids = Array.ConvertAll(regroupPlayers.Split(','), value => long.Parse(value.Trim())) }));
		GUILayout.Label("续组申请 ID"); regroupId = GUILayout.TextField(regroupId);
		if (GUILayout.Button("查询续组状态")) Invoke(async () => { var response = await client.Query("/api/v2/regroup/" + Uri.EscapeDataString(regroupId)); regroupRevision = (long?)response["data"]?["revision"] ?? 1; status = response.ToString(); });
		if (GUILayout.Button("同意续组")) Invoke(() => client.Send("v2.party.regroup_respond", new { proposal_id = regroupId, revision = regroupRevision, accept = true }));
		if (GUILayout.Button("拒绝续组")) Invoke(() => client.Send("v2.party.regroup_respond", new { proposal_id = regroupId, revision = regroupRevision, accept = false }));
        GUI.enabled = true;
        GUILayout.TextArea(status);
        GUILayout.EndScrollView();
        GUILayout.EndArea();
    }
}
