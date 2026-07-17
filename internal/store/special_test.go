package store

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestDescribeSpecialMessagePoll(t *testing.T) {
	msg := &waE2E.Message{
		PollCreationMessageV3: &waE2E.PollCreationMessage{
			Name: proto.String("Lunch <spot>?"),
			Options: []*waE2E.PollCreationMessage_Option{
				{OptionName: proto.String("Pizza")},
				{OptionName: proto.String("Sushi & more")},
			},
		},
	}
	out, ok := DescribeSpecialMessage(msg)
	if !ok {
		t.Fatal("poll not recognized")
	}
	for _, want := range []string{"Lunch &lt;spot&gt;?", "Pizza", "Sushi &amp; more", "msg-poll"} {
		if !strings.Contains(out, want) {
			t.Errorf("poll html missing %q in %q", want, out)
		}
	}
}

func TestDescribeSpecialMessageLocation(t *testing.T) {
	msg := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(12.9716),
			DegreesLongitude: proto.Float64(77.5946),
			Name:             proto.String("MG Road"),
		},
	}
	out, ok := DescribeSpecialMessage(msg)
	if !ok {
		t.Fatal("location not recognized")
	}
	if !strings.Contains(out, "google.com/maps") || !strings.Contains(out, "MG Road") {
		t.Errorf("location html wrong: %q", out)
	}
}

func TestDescribeSpecialMessageContactExtractsPhone(t *testing.T) {
	msg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String("Alice"),
			Vcard:       proto.String("BEGIN:VCARD\nVERSION:3.0\nFN:Alice\nTEL;type=CELL:+91 98765 43210\nEND:VCARD"),
		},
	}
	out, ok := DescribeSpecialMessage(msg)
	if !ok {
		t.Fatal("contact not recognized")
	}
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "+91 98765 43210") {
		t.Errorf("contact html wrong: %q", out)
	}
}

func TestDescribeSpecialMessageGroupInvite(t *testing.T) {
	msg := &waE2E.Message{
		GroupInviteMessage: &waE2E.GroupInviteMessage{
			GroupName:  proto.String("Linux Forum"),
			InviteCode: proto.String("AbC123"),
		},
	}
	out, ok := DescribeSpecialMessage(msg)
	if !ok {
		t.Fatal("invite not recognized")
	}
	if !strings.Contains(out, "chat.whatsapp.com/AbC123") || !strings.Contains(out, "Linux Forum") {
		t.Errorf("invite html wrong: %q", out)
	}
}

func TestDescribeSpecialMessageUnknownFallsThrough(t *testing.T) {
	if out, ok := DescribeSpecialMessage(&waE2E.Message{}); ok {
		t.Errorf("empty message should not be special, got %q", out)
	}
	if _, ok := DescribeSpecialMessage(nil); ok {
		t.Error("nil message should not be special")
	}
}

func TestSpecialPreview(t *testing.T) {
	poll := &waE2E.Message{
		PollCreationMessage: &waE2E.PollCreationMessage{Name: proto.String("Q")},
	}
	if p, ok := SpecialPreview(poll); !ok || p != "📊 Q" {
		t.Errorf("poll preview = %q ok=%v", p, ok)
	}
	ptv := &waE2E.Message{PtvMessage: &waE2E.VideoMessage{}}
	if p, ok := SpecialPreview(ptv); !ok || p != "🎥 Video note" {
		t.Errorf("ptv preview = %q ok=%v", p, ok)
	}
	if _, ok := SpecialPreview(&waE2E.Message{}); ok {
		t.Error("empty message should have no special preview")
	}
}

func TestShouldSkipMessage(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		want bool
	}{
		{"nil", nil, true},
		{"poll update", &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}}, true},
		{"keep in chat", &waE2E.Message{KeepInChatMessage: &waE2E.KeepInChatMessage{}}, true},
		{"protocol", &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}, true},
		{"plain text", &waE2E.Message{Conversation: proto.String("hi")}, false},
		{"poll creation", &waE2E.Message{
			PollCreationMessage: &waE2E.PollCreationMessage{Name: proto.String("Q")},
		}, false},
	}
	for _, tc := range cases {
		if got := ShouldSkipMessage(tc.msg); got != tc.want {
			t.Errorf("%s: ShouldSkipMessage = %v, want %v", tc.name, got, tc.want)
		}
	}
}
