package matchmaking

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name     string
		current  Status
		target   Status
		expected bool
	}{
		{name: "queued to canceled", current: StatusQueued, target: StatusCanceled, expected: true},
		{name: "queued to timeout", current: StatusQueued, target: StatusTimeout, expected: true},
		{name: "canceled to queued", current: StatusCanceled, target: StatusQueued, expected: false},
		{name: "timeout to canceled", current: StatusTimeout, target: StatusCanceled, expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := canTransition(test.current, test.target); actual != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, actual)
			}
		})
	}
}

func TestValidIdentifier(t *testing.T) {
	valid := []string{"training_ground", "mission-01", "damage.any"}
	for _, value := range valid {
		if !validIdentifier(value, 64) {
			t.Fatalf("expected %q to be valid", value)
		}
	}

	invalid := []string{"", "mission:01", "包含中文", "role space"}
	for _, value := range invalid {
		if validIdentifier(value, 64) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestTimeoutMemberRoundTrip(t *testing.T) {
	ticket := &Ticket{ID: "ticket_1", MissionID: "training_ground"}
	value := timeoutMember(ticket)
	missionID, ticketID, ok := parseTimeoutMember(value)
	if !ok {
		t.Fatal("timeout member should parse")
	}
	if missionID != ticket.MissionID || ticketID != ticket.ID {
		t.Fatalf("unexpected result: mission_id=%s ticket_id=%s", missionID, ticketID)
	}
}
