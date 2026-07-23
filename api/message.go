package api

import (
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lugvitc/whats4linux/internal/markdown"
	"github.com/lugvitc/whats4linux/internal/store"
	mtypes "github.com/lugvitc/whats4linux/internal/types"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type MessageContent struct {
	Type            string   `json:"type"`
	Text            string   `json:"text,omitempty"`
	Base64Data      string   `json:"base64Data,omitempty"`
	Mimetype        string   `json:"mimetype,omitempty"`
	FileName        string   `json:"fileName,omitempty"`
	QuotedMessageID string   `json:"quotedMessageId,omitempty"`
	Mentions        []string `json:"mentions,omitempty"`
	ClientTempID    string   `json:"clientTempId,omitempty"`
}

func (a *Api) processMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	var text string
	var mentionedJIDs []string

	if msg.GetConversation() != "" {
		text = msg.GetConversation()
	} else if msg.GetExtendedTextMessage() != nil {
		text = msg.GetExtendedTextMessage().GetText()
		if msg.GetExtendedTextMessage().GetContextInfo() != nil {
			mentionedJIDs = msg.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()
		}
	} else {
		switch {
		case msg.GetImageMessage() != nil:
			text = msg.GetImageMessage().GetCaption()
			if msg.GetImageMessage().GetContextInfo() != nil {
				mentionedJIDs = msg.GetImageMessage().GetContextInfo().GetMentionedJID()
			}
		case msg.GetVideoMessage() != nil:
			text = msg.GetVideoMessage().GetCaption()
			if msg.GetVideoMessage().GetContextInfo() != nil {
				mentionedJIDs = msg.GetVideoMessage().GetContextInfo().GetMentionedJID()
			}
		case msg.GetDocumentMessage() != nil:
			text = msg.GetDocumentMessage().GetCaption()
			if msg.GetDocumentMessage().GetContextInfo() != nil {
				mentionedJIDs = msg.GetDocumentMessage().GetContextInfo().GetMentionedJID()
			}
		}
	}

	if text == "" {
		return ""
	}

	// First convert Markdown to HTML (which handles escaping)
	htmlText := markdown.MarkdownLinesToHTML(text)

	// Then replace mentions in the HTML
	if len(mentionedJIDs) > 0 {
		htmlText = replaceMentions(htmlText, mentionedJIDs, a)
	}

	return htmlText
}

func (a *Api) FetchMessagesPaged(jid string, limit int, beforeTimestamp int64, beforeMessageID string) ([]store.DecodedMessage, error) {
	messages, err := a.messageStore.GetDecodedMessagesPaged(jid, beforeTimestamp, beforeMessageID, limit)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// brRE turns <br> tags into newlines when converting stored HTML back to text.
var brRE = regexp.MustCompile(`(?i)<br\s*/?>`)

// htmlToPlain converts the stored HTML message text back to plain text so a
// forwarded message doesn't arrive full of markup. (The DB keeps the parsed
// HTML used for rendering, not the original plain text.)
func htmlToPlain(s string) string {
	s = brRE.ReplaceAllString(s, "\n")
	s = htmlTagRE.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// ForwardMessage forwards a stored message to another chat. Media is
// re-downloaded and re-uploaded (forward-by-reference proved unreliable) and
// text/captions are converted back to plain text. The forwarded flag is set.
func (a *Api) ForwardMessage(fromChatJID, messageID, toChatJID string) error {
	toJID, err := types.ParseJID(toChatJID)
	if err != nil {
		return err
	}
	src, err := a.messageStore.GetMessageWithMedia(fromChatJID, messageID)
	if err != nil || src == nil {
		return fmt.Errorf("message not found")
	}

	fwd := &waE2E.ContextInfo{
		IsForwarded:     proto.Bool(true),
		ForwardingScore: proto.Uint32(1),
	}
	caption := htmlToPlain(src.Text)

	var msg *waE2E.Message

	if src.Media == nil {
		if caption == "" {
			return fmt.Errorf("nothing to forward")
		}
		msg = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(caption), ContextInfo: fwd,
		}}
	} else {
		data, err := a.waClient.Download(a.ctx, src.Media)
		if err != nil {
			return fmt.Errorf("download failed: %v", err)
		}
		mediaType := src.Media.GetMediaType()
		uploaded, err := a.waClient.Upload(a.ctx, data, mediaType)
		if err != nil {
			return fmt.Errorf("upload failed: %v", err)
		}
		mime := src.Media.GetMimetype()

		switch mediaType {
		case whatsmeow.MediaImage:
			if mime == "" {
				mime = "image/jpeg"
			}
			w, h := src.Media.GetDimensions()
			msg = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
				URL: &uploaded.URL, DirectPath: &uploaded.DirectPath, MediaKey: uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256, FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength, Mimetype: &mime, Caption: proto.String(caption),
				Width: proto.Uint32(uint32(w)), Height: proto.Uint32(uint32(h)), ContextInfo: fwd,
			}}
		case whatsmeow.MediaVideo:
			if mime == "" {
				mime = "video/mp4"
			}
			msg = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
				URL: &uploaded.URL, DirectPath: &uploaded.DirectPath, MediaKey: uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256, FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength, Mimetype: &mime, Caption: proto.String(caption),
				ContextInfo: fwd,
			}}
		case whatsmeow.MediaAudio:
			if mime == "" {
				mime = "audio/ogg"
			}
			msg = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
				URL: &uploaded.URL, DirectPath: &uploaded.DirectPath, MediaKey: uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256, FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength, Mimetype: &mime, ContextInfo: fwd,
			}}
		case whatsmeow.MediaDocument:
			if mime == "" {
				mime = "application/octet-stream"
			}
			doc := &waE2E.DocumentMessage{
				URL: &uploaded.URL, DirectPath: &uploaded.DirectPath, MediaKey: uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256, FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength, Mimetype: &mime, Caption: proto.String(caption),
				ContextInfo: fwd,
			}
			if dec, derr := a.messageStore.GetDecodedMessage(fromChatJID, messageID); derr == nil &&
				dec.Content != nil && dec.Content.DocumentMessage != nil && dec.Content.DocumentMessage.FileName != "" {
				doc.FileName = proto.String(dec.Content.DocumentMessage.FileName)
			}
			msg = &waE2E.Message{DocumentMessage: doc}
		default:
			return fmt.Errorf("cannot forward this media type")
		}
	}

	_, err = a.waClient.SendMessage(a.ctx, toJID, msg)
	return err
}

// probeDurationSeconds returns the rounded duration of a media file (0 on error).
func probeDurationSeconds(path string) uint32 {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || f < 0 {
		return 0
	}
	return uint32(f + 0.5)
}

// computeWaveform builds WhatsApp's 64-sample 0-100 amplitude envelope from raw
// signed-16-bit mono PCM, so the voice note renders with a proper waveform.
func computeWaveform(pcm []byte) []byte {
	const buckets = 64
	n := len(pcm) / 2
	if n == 0 {
		return nil
	}
	amps := make([]float64, buckets)
	per := n / buckets
	if per == 0 {
		per = 1
	}
	var maxAmp float64
	for b := 0; b < buckets; b++ {
		start := b * per
		end := start + per
		if end > n {
			end = n
		}
		var sum float64
		cnt := 0
		for i := start; i < end; i++ {
			s := int16(uint16(pcm[2*i]) | uint16(pcm[2*i+1])<<8)
			v := float64(s)
			if v < 0 {
				v = -v
			}
			sum += v
			cnt++
		}
		if cnt > 0 {
			amps[b] = sum / float64(cnt)
		}
		if amps[b] > maxAmp {
			maxAmp = amps[b]
		}
	}
	wf := make([]byte, buckets)
	for b := 0; b < buckets; b++ {
		if maxAmp > 0 {
			wf[b] = byte(amps[b] / maxAmp * 100)
		}
	}
	return wf
}

// StartVoiceRecording begins capturing the default system microphone with
// ffmpeg as raw PCM (no container, so a stop can't corrupt it). The final
// Opus/Ogg encode happens in StopVoiceRecording. We record on the backend
// because WebKitGTK denies the WebView getUserMedia permission.
func (a *Api) StartVoiceRecording() error {
	a.voiceMu.Lock()
	defer a.voiceMu.Unlock()
	if a.voiceCmd != nil {
		return fmt.Errorf("already recording")
	}

	f, err := os.CreateTemp("", "w4l-rec-*.pcm")
	if err != nil {
		return err
	}
	path := f.Name()
	f.Close()

	cmd := exec.Command("ffmpeg", "-y",
		"-f", "pulse", "-i", "default",
		"-f", "s16le", "-ac", "1", "-ar", "48000", path)
	cmd.Stderr = os.Stderr // surface ffmpeg errors in the app log
	if err := cmd.Start(); err != nil {
		os.Remove(path)
		return fmt.Errorf("failed to start recorder: %v", err)
	}

	log.Printf("voice: recording to %s (ffmpeg pid %d)", path, cmd.Process.Pid)
	a.voiceCmd = cmd
	a.voicePath = path
	return nil
}

// CancelVoiceRecording stops and discards an in-progress recording.
func (a *Api) CancelVoiceRecording() error {
	a.voiceMu.Lock()
	defer a.voiceMu.Unlock()
	if a.voiceCmd == nil {
		return nil
	}
	_ = a.voiceCmd.Process.Kill()
	_, _ = a.voiceCmd.Process.Wait()
	os.Remove(a.voicePath)
	a.voiceCmd = nil
	a.voicePath = ""
	return nil
}

// StopVoiceRecording finalises the recording and sends it as a voice note (PTT).
// StopVoiceRecording finalises the recording, sends it as a voice note (PTT),
// and returns the audio as a data URL so the desktop UI can play it back
// locally (our own sent messages aren't re-fetched from the server).
func (a *Api) StopVoiceRecording(chatJID string) (string, error) {
	a.voiceMu.Lock()
	cmd := a.voiceCmd
	path := a.voicePath
	a.voiceCmd = nil
	a.voicePath = ""
	a.voiceMu.Unlock()

	if cmd == nil {
		return "", fmt.Errorf("not recording")
	}
	defer os.Remove(path)

	// Stop the capture. The recording is raw PCM, so a truncated tail is
	// harmless (there's no container trailer to corrupt).
	_ = cmd.Process.Signal(os.Interrupt)
	_ = cmd.Wait()

	if a.waClient.Store.ID == nil {
		return "", fmt.Errorf("client not logged in")
	}
	toJID, err := types.ParseJID(chatJID)
	if err != nil {
		return "", err
	}

	pcm, err := os.ReadFile(path)
	if err != nil || len(pcm) == 0 {
		log.Printf("voice: no audio captured (%s, err=%v, bytes=%d)", path, err, len(pcm))
		return "", fmt.Errorf("no audio captured")
	}

	// Encode the complete recording to Opus/Ogg in one clean pass (voip mode,
	// as WhatsApp voice notes use), guaranteeing a well-formed file.
	oggPath := path + ".ogg"
	defer os.Remove(oggPath)
	enc := exec.Command("ffmpeg", "-y",
		"-f", "s16le", "-ac", "1", "-ar", "48000", "-i", path,
		"-c:a", "libopus", "-b:a", "32k", "-application", "voip", oggPath)
	enc.Stderr = os.Stderr
	if err := enc.Run(); err != nil {
		log.Printf("voice: encode failed: %v", err)
		return "", fmt.Errorf("failed to encode voice note: %v", err)
	}

	ogg, err := os.ReadFile(oggPath)
	if err != nil || len(ogg) == 0 {
		return "", fmt.Errorf("encode produced no output")
	}
	seconds := probeDurationSeconds(oggPath)
	waveform := computeWaveform(pcm)
	log.Printf("voice: encoded %d bytes, %ds; uploading", len(ogg), seconds)

	uploaded, err := a.waClient.Upload(a.ctx, ogg, whatsmeow.MediaAudio)
	if err != nil {
		log.Printf("voice: upload failed: %v", err)
		return "", fmt.Errorf("failed to upload voice note: %v", err)
	}
	mime := "audio/ogg; codecs=opus"
	msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		URL: &uploaded.URL, DirectPath: &uploaded.DirectPath, MediaKey: uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256, FileSHA256: uploaded.FileSHA256,
		FileLength: &uploaded.FileLength, Mimetype: &mime,
		// WhatsApp mobile validates MediaKeyTimestamp; without it the media can
		// show as "no longer available" even when the blob is retrievable.
		MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
		Seconds:           proto.Uint32(seconds),
		PTT:               proto.Bool(true),
		Waveform:          waveform,
	}}

	if _, err = a.waClient.SendMessage(a.ctx, toJID, msg); err != nil {
		log.Printf("voice: send failed: %v", err)
		return "", err
	}
	log.Printf("voice: sent to %s", chatJID)
	return "data:audio/ogg;base64," + base64.StdEncoding.EncodeToString(ogg), nil
}

// DeleteMessageForMe removes a message from the local database only (the other
// participants keep their copy).
func (a *Api) DeleteMessageForMe(chatJID, messageID string) error {
	if err := a.messageStore.DeleteMessage(messageID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "wa:message_deleted", map[string]any{
		"chatId":    chatJID,
		"messageId": messageID,
	})
	return nil
}

// DeleteMessageForEveryone revokes one of our own messages for all participants
// and removes it locally.
func (a *Api) DeleteMessageForEveryone(chatJID, messageID string) error {
	parsedJID, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	if _, err := a.waClient.RevokeMessage(a.ctx, parsedJID, messageID); err != nil {
		return fmt.Errorf("revoke failed: %v", err)
	}
	if err := a.messageStore.DeleteMessage(messageID); err != nil {
		log.Println("revoke: local delete failed:", err)
	}
	runtime.EventsEmit(a.ctx, "wa:message_deleted", map[string]any{
		"chatId":    chatJID,
		"messageId": messageID,
	})
	return nil
}

// SearchMessages returns messages in a chat whose text matches the query
// (newest first). Blank queries return nothing.
func (a *Api) SearchMessages(chatJID, queryText string, limit int) ([]store.DecodedMessage, error) {
	if len(queryText) == 0 {
		return []store.DecodedMessage{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	return a.messageStore.SearchDecodedMessages(chatJID, queryText, limit)
}

// FetchMessagesAround returns a window of messages centred on messageID so the
// frontend can display a search result in its surrounding context.
func (a *Api) FetchMessagesAround(chatJID, messageID string, limit int) ([]store.DecodedMessage, error) {
	if limit <= 0 {
		limit = 25
	}
	return a.messageStore.GetDecodedMessagesAround(chatJID, messageID, limit)
}

// SearchChatJIDs returns chat JIDs (most-recent match first) containing a
// message matching the query. Powers global content search in the chat list.
func (a *Api) SearchChatJIDs(queryText string, limit int) ([]string, error) {
	if len(queryText) == 0 {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	return a.messageStore.SearchChatJIDsByMessage(queryText, limit)
}

func buildQuotedMessage(msg *store.ExtendedMessage) *waE2E.Message {
	if msg == nil {
		return nil
	}
	var quotedMessage waE2E.Message
	if msg.ReplyToMessageID == "" {
		quotedMessage.Conversation = proto.String(msg.Text)
	} else {
		quotedMessage.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text: proto.String(msg.Text),
		}
	}

	if msg.Media == nil {
		return &quotedMessage
	}

	switch msg.Media.GetMediaGeneralType() {
	case mtypes.MediaTypeImage:
		width, height := msg.Media.GetDimensions()
		quotedMessage.ImageMessage = &waE2E.ImageMessage{
			URL:           proto.String(msg.Media.GetURL()),
			Mimetype:      proto.String(msg.Media.GetMimetype()),
			Caption:       proto.String(msg.Text),
			FileSHA256:    msg.Media.GetFileSHA256(),
			Width:         proto.Uint32(uint32(width)),
			Height:        proto.Uint32(uint32(height)),
			FileEncSHA256: msg.Media.GetFileEncSHA256(),
			DirectPath:    proto.String(msg.Media.GetDirectPath()),
		}
	case mtypes.MediaTypeVideo:
		quotedMessage.VideoMessage = &waE2E.VideoMessage{
			URL:           proto.String(msg.Media.GetURL()),
			Mimetype:      proto.String(msg.Media.GetMimetype()),
			Caption:       proto.String(msg.Text),
			FileSHA256:    msg.Media.GetFileSHA256(),
			FileEncSHA256: msg.Media.GetFileEncSHA256(),
			DirectPath:    proto.String(msg.Media.GetDirectPath()),
		}
	case mtypes.MediaTypeAudio:
		quotedMessage.AudioMessage = &waE2E.AudioMessage{
			URL:           proto.String(msg.Media.GetURL()),
			Mimetype:      proto.String(msg.Media.GetMimetype()),
			FileSHA256:    msg.Media.GetFileSHA256(),
			FileEncSHA256: msg.Media.GetFileEncSHA256(),
			DirectPath:    proto.String(msg.Media.GetDirectPath()),
		}
	case mtypes.MediaTypeDocument:
		quotedMessage.DocumentMessage = &waE2E.DocumentMessage{
			URL:           proto.String(msg.Media.GetURL()),
			Mimetype:      proto.String(msg.Media.GetMimetype()),
			Caption:       proto.String(msg.Text),
			FileSHA256:    msg.Media.GetFileSHA256(),
			FileEncSHA256: msg.Media.GetFileEncSHA256(),
			DirectPath:    proto.String(msg.Media.GetDirectPath()),
		}
	case mtypes.MediaTypeSticker:
		quotedMessage.StickerMessage = &waE2E.StickerMessage{
			URL:           proto.String(msg.Media.GetURL()),
			Mimetype:      proto.String(msg.Media.GetMimetype()),
			FileSHA256:    msg.Media.GetFileSHA256(),
			FileEncSHA256: msg.Media.GetFileEncSHA256(),
			DirectPath:    proto.String(msg.Media.GetDirectPath()),
		}
	}

	return &quotedMessage
}

func (a *Api) buildQuotedContext(chatJID types.JID, quotedMessageID string) (*waE2E.ContextInfo, error) {
	if quotedMessageID == "" {
		return nil, nil
	}

	msg, err := a.messageStore.GetMessageWithMedia(chatJID.String(), quotedMessageID)
	if err != nil {
		return nil, fmt.Errorf("quoted message not found")
	}

	quotedMessage := buildQuotedMessage(msg)

	if quotedMessage == nil {
		return nil, fmt.Errorf("failed to build quoted message")
	}

	stanzaID := quotedMessageID
	contextInfo := &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		QuotedMessage: quotedMessage,
	}

	if msg.Info.Sender.User != "" {
		participantJID := msg.Info.Sender.ToNonAD().String()
		contextInfo.Participant = proto.String(participantJID)
	}

	return contextInfo, nil
}

// SendReaction reacts to a message with an emoji (empty emoji removes the
// reaction). senderJID is the original message's sender; empty means our own.
func (a *Api) SendReaction(chatJID, senderJID, messageID, emoji string) error {
	if a.waClient.Store.ID == nil {
		return fmt.Errorf("not logged in")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	sender := *a.waClient.Store.ID
	if senderJID != "" {
		if s, perr := types.ParseJID(senderJID); perr == nil {
			sender = s
		}
	}
	reactionMsg := a.waClient.BuildReaction(chat, sender, messageID, emoji)
	if _, err := a.waClient.SendMessage(a.ctx, chat, reactionMsg); err != nil {
		return err
	}
	// Persist our own reaction locally so it survives a reload.
	_ = a.messageStore.AddReactionToMessage(messageID, emoji, a.waClient.Store.ID.String())
	return nil
}

func (a *Api) SendMessage(chatJID string, content MessageContent) (string, error) {
	if a.waClient.Store.ID == nil {
		return "", fmt.Errorf("client not logged in")
	}

	parsedJID, err := types.ParseJID(chatJID)
	if err != nil {
		return "", err
	}

	var msgContent *waE2E.Message
	contextInfo, err := a.buildQuotedContext(parsedJID, content.QuotedMessageID)
	if err != nil {
		log.Println("Failed to build quoted context:", err)
		return "", err
	}

	switch content.Type {
	case "text":

		mentionedJIDs := content.Mentions

		// Fetch a link preview if the message contains a URL, so the card shows
		// for both sender and recipient (WhatsApp attaches it to the message).
		var lm *linkMeta
		if u := urlRE.FindString(content.Text); u != "" {
			lm = fetchLinkMeta(u)
		}

		// If we have mentions, quoted context, or a link preview, use
		// ExtendedTextMessage.
		if len(mentionedJIDs) > 0 || contextInfo != nil || lm != nil {
			if contextInfo == nil {
				contextInfo = &waE2E.ContextInfo{}
			}
			if len(mentionedJIDs) > 0 {
				contextInfo.MentionedJID = mentionedJIDs
			}
			etm := &waE2E.ExtendedTextMessage{
				Text:        &content.Text,
				ContextInfo: contextInfo,
			}
			if lm != nil {
				etm.MatchedText = proto.String(lm.url)
				if lm.title != "" {
					etm.Title = proto.String(lm.title)
				}
				if lm.description != "" {
					etm.Description = proto.String(lm.description)
				}
				if len(lm.thumbnail) > 0 {
					etm.JPEGThumbnail = lm.thumbnail
				}
			}
			msgContent = &waE2E.Message{ExtendedTextMessage: etm}
		} else {
			msgContent = &waE2E.Message{
				Conversation: &content.Text,
			}
		}
	case "image":
		// Decode base64 image data
		imageData, err := base64.StdEncoding.DecodeString(content.Base64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 image data: %v", err)
		}

		// Create image message
		mimeType := content.Mimetype
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
		imageMsg := &waE2E.ImageMessage{
			Mimetype:      &mimeType,
			Caption:       &content.Text,
			JPEGThumbnail: nil, // We'll let WhatsApp generate the thumbnail
		}

		if len(content.Mentions) > 0 || contextInfo != nil {
			if contextInfo == nil {
				contextInfo = &waE2E.ContextInfo{}
			}
			if len(content.Mentions) > 0 {
				contextInfo.MentionedJID = content.Mentions
			}
			imageMsg.ContextInfo = contextInfo
		}

		// Upload the image
		uploaded, err := a.waClient.Upload(a.ctx, imageData, whatsmeow.MediaImage)
		if err != nil {
			return "", fmt.Errorf("failed to upload image: %v", err)
		}

		imageMsg.URL = &uploaded.URL
		imageMsg.DirectPath = &uploaded.DirectPath
		imageMsg.MediaKey = uploaded.MediaKey
		imageMsg.FileEncSHA256 = uploaded.FileEncSHA256
		imageMsg.FileSHA256 = uploaded.FileSHA256
		imageMsg.FileLength = &uploaded.FileLength

		msgContent = &waE2E.Message{
			ImageMessage: imageMsg,
		}
	case "video":
		// Decode base64 video data
		videoData, err := base64.StdEncoding.DecodeString(content.Base64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 video data: %v", err)
		}

		// Create video message
		mimeType := content.Mimetype
		if mimeType == "" {
			mimeType = "video/mp4"
		}
		videoMsg := &waE2E.VideoMessage{
			Mimetype:      &mimeType,
			Caption:       &content.Text,
			JPEGThumbnail: nil, // We'll let WhatsApp generate the thumbnail
		}

		if len(content.Mentions) > 0 || contextInfo != nil {
			if contextInfo == nil {
				contextInfo = &waE2E.ContextInfo{}
			}
			if len(content.Mentions) > 0 {
				contextInfo.MentionedJID = content.Mentions
			}
			videoMsg.ContextInfo = contextInfo
		}

		// Upload the video
		uploaded, err := a.waClient.Upload(a.ctx, videoData, whatsmeow.MediaVideo)
		if err != nil {
			return "", fmt.Errorf("failed to upload video: %v", err)
		}

		videoMsg.URL = &uploaded.URL
		videoMsg.DirectPath = &uploaded.DirectPath
		videoMsg.MediaKey = uploaded.MediaKey
		videoMsg.FileEncSHA256 = uploaded.FileEncSHA256
		videoMsg.FileSHA256 = uploaded.FileSHA256
		videoMsg.FileLength = &uploaded.FileLength

		msgContent = &waE2E.Message{
			VideoMessage: videoMsg,
		}
	case "audio":
		// Decode base64 audio data
		audioData, err := base64.StdEncoding.DecodeString(content.Base64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 audio data: %v", err)
		}

		// Create audio message
		mimeType := content.Mimetype
		if mimeType == "" {
			mimeType = "audio/ogg"
		}
		audioMsg := &waE2E.AudioMessage{
			Mimetype: &mimeType,
		}

		if contextInfo != nil {
			audioMsg.ContextInfo = contextInfo
		}

		// Upload the audio
		uploaded, err := a.waClient.Upload(a.ctx, audioData, whatsmeow.MediaAudio)
		if err != nil {
			return "", fmt.Errorf("failed to upload audio: %v", err)
		}

		audioMsg.URL = &uploaded.URL
		audioMsg.DirectPath = &uploaded.DirectPath
		audioMsg.MediaKey = uploaded.MediaKey
		audioMsg.FileEncSHA256 = uploaded.FileEncSHA256
		audioMsg.FileSHA256 = uploaded.FileSHA256
		audioMsg.FileLength = &uploaded.FileLength

		msgContent = &waE2E.Message{
			AudioMessage: audioMsg,
		}
	case "document":
		// Decode base64 document data
		documentData, err := base64.StdEncoding.DecodeString(content.Base64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 document data: %v", err)
		}

		// Create document message
		mimeType := content.Mimetype
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		fileName := strings.TrimSpace(content.FileName)
		if fileName == "" {
			fileName = "document"
		}
		documentMsg := &waE2E.DocumentMessage{
			Mimetype: &mimeType,
			FileName: &fileName,
			Caption:  &content.Text,
		}

		if len(content.Mentions) > 0 || contextInfo != nil {
			if contextInfo == nil {
				contextInfo = &waE2E.ContextInfo{}
			}
			if len(content.Mentions) > 0 {
				contextInfo.MentionedJID = content.Mentions
			}
			documentMsg.ContextInfo = contextInfo
		}

		// Upload the document
		uploaded, err := a.waClient.Upload(a.ctx, documentData, whatsmeow.MediaDocument)
		if err != nil {
			return "", fmt.Errorf("failed to upload document: %v", err)
		}

		documentMsg.URL = &uploaded.URL
		documentMsg.DirectPath = &uploaded.DirectPath
		documentMsg.MediaKey = uploaded.MediaKey
		documentMsg.FileEncSHA256 = uploaded.FileEncSHA256
		documentMsg.FileSHA256 = uploaded.FileSHA256
		documentMsg.FileLength = &uploaded.FileLength

		msgContent = &waE2E.Message{
			DocumentMessage: documentMsg,
		}
	case "sticker":
		// Decode base64 sticker data
		stickerData, err := base64.StdEncoding.DecodeString(content.Base64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 sticker data: %v", err)
		}

		// Create sticker message
		mimeType := content.Mimetype
		if mimeType == "" {
			mimeType = "image/webp"
		}
		stickerMsg := &waE2E.StickerMessage{
			Mimetype: &mimeType,
		}

		// Upload the sticker
		uploaded, err := a.waClient.Upload(a.ctx, stickerData, whatsmeow.MediaImage) // Stickers use MediaImage
		if err != nil {
			return "", fmt.Errorf("failed to upload sticker: %v", err)
		}

		stickerMsg.URL = &uploaded.URL
		stickerMsg.DirectPath = &uploaded.DirectPath
		stickerMsg.MediaKey = uploaded.MediaKey
		stickerMsg.FileEncSHA256 = uploaded.FileEncSHA256
		stickerMsg.FileSHA256 = uploaded.FileSHA256
		stickerMsg.FileLength = &uploaded.FileLength

		msgContent = &waE2E.Message{
			StickerMessage: stickerMsg,
		}
	default:
		return "", fmt.Errorf("unsupported message type: %s", content.Type)
	}

	log.Printf("SendMessage Content: %+v\n", msgContent)

	resp, err := a.waClient.SendMessage(a.ctx, parsedJID, msgContent)
	if err != nil {
		log.Println("SendMessage error:", err)
		return "", err
	}

	// Manually add to store and emit event so UI updates immediately
	msgEvent := &events.Message{
		Info: types.MessageInfo{
			ID:        resp.ID,
			Timestamp: resp.Timestamp,
			MessageSource: types.MessageSource{
				Chat:     parsedJID,
				IsFromMe: true,
				Sender:   *a.waClient.Store.ID,
			},
		},
		Message: msgContent,
	}
	parsedHTML := a.processMessageText(msgContent)
	messageID := a.messageStore.ProcessMessageEvent(a.ctx, a.waClient.Store.LIDs, msgEvent, parsedHTML)

	// Extract message text for chat list update
	var messageText string
	if msgContent.GetConversation() != "" {
		messageText = msgContent.GetConversation()
	} else if msgContent.GetExtendedTextMessage() != nil {
		messageText = msgContent.GetExtendedTextMessage().GetText()
	} else {
		switch {
		case msgContent.GetImageMessage() != nil:
			messageText = "image"
		case msgContent.GetVideoMessage() != nil:
			messageText = "video"
		case msgContent.GetAudioMessage() != nil:
			messageText = "audio"
		case msgContent.GetDocumentMessage() != nil:
			messageText = "document"
		case msgContent.GetStickerMessage() != nil:
			messageText = "sticker"
		default:
			messageText = "message"
		}
	}

	var msg any
	if messageID != "" {
		decodedMsg, err := a.messageStore.GetDecodedMessage(parsedJID.String(), messageID)
		if err == nil {
			msg = decodedMsg
		}
	}

	if msg == nil {
		msg = struct {
			Info    types.MessageInfo
			Content *waE2E.Message
		}{
			Info:    msgEvent.Info,
			Content: msgEvent.Message,
		}
	}

	runtime.EventsEmit(a.ctx, "wa:new_message", map[string]any{
		"chatId":       parsedJID.String(),
		"message":      msg,
		"clientTempId": content.ClientTempID,
		"messageText":  messageText,
		"parsedHTML":   parsedHTML,
		"timestamp":    resp.Timestamp.Unix(),
		"sender":       "You",
	})

	return resp.ID, nil
}
// readReceiptsEnabled reports whether the user currently allows sending read
// receipts. It defaults to true when the setting is missing or malformed, which
// matches the frontend default and WhatsApp's out-of-the-box behaviour.
func readReceiptsEnabled() bool {
	v, ok := store.GetSettings()["readReceipts"]
	if !ok {
		return true
	}
	enabled, ok := v.(bool)
	if !ok {
		return true
	}
	return enabled
}

func (a *Api) MarkRead(chatJID string, messageIDs []string, Type string) error {
	parsedChatJID, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	if Type == "read-msg" {
		// Reading here advances our read watermark regardless of the read-receipt
		// privacy setting: withholding a receipt from the sender doesn't mean the
		// messages are still unread for us. Advance to the newest incoming
		// message so the whole chat reads as read.
		canonical := a.canonicalJID(parsedChatJID)
		if ts := a.messageStore.NewestIncomingTimestamp(canonical); ts > 0 {
			a.emitUnread(canonical, a.messageStore.AdvanceReadWatermark(canonical, ts))
		}

		// Honour the read-receipt privacy setting. When it's off we never tell
		// WhatsApp we've seen the message, so the sender keeps seeing the
		// delivered (gray) ticks instead of a read confirmation we didn't make.
		// WhatsApp then reciprocally withholds others' read receipts from us.
		if !readReceiptsEnabled() {
			return nil
		}
		for _, msgID := range messageIDs {
			msg, err := a.messageStore.GetMessageWithMedia(chatJID, msgID)
			if err != nil {
				log.Printf("Failed to get message %s: %v", msgID, err)
				continue
			}
			senderJID := msg.Info.Sender
			ids := []types.MessageID{types.MessageID(msgID)}
			err = a.waClient.MarkRead(a.ctx, ids, time.Now(), parsedChatJID, senderJID)
			if err != nil {
				log.Printf("MarkRead error for message %s: %v", msgID, err)
			}
		}
	}
	return nil
}

// ---- Message pins ----

// PinExpirySeconds mirrors WhatsApp's default pin duration (7 days).
const PinExpirySeconds = 7 * 24 * 60 * 60

func (a *Api) GetPinnedMessages(chatJID string) ([]store.PinnedMessage, error) {
	return a.messageStore.GetPinnedMessages(chatJID)
}

// SetMessagePinned pins or unpins a message for everyone in the chat and
// records the change locally.
func (a *Api) SetMessagePinned(chatJID, senderJID, messageID string, fromMe, pin bool) error {
	if a.waClient.Store.ID == nil {
		return fmt.Errorf("not logged in")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(chatJID),
		FromMe:    proto.Bool(fromMe),
		ID:        proto.String(messageID),
	}
	if chat.Server == types.GroupServer && !fromMe && senderJID != "" {
		key.Participant = proto.String(senderJID)
	}

	pinType := waE2E.PinInChatMessage_PIN_FOR_ALL
	if !pin {
		pinType = waE2E.PinInChatMessage_UNPIN_FOR_ALL
	}
	msg := &waE2E.Message{
		PinInChatMessage: &waE2E.PinInChatMessage{
			Key:               key,
			Type:              &pinType,
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageAddOnDurationInSecs: proto.Uint32(uint32(PinExpirySeconds)),
		},
	}
	if _, err := a.waClient.SendMessage(a.ctx, chat, msg); err != nil {
		return err
	}

	sender := a.waClient.Store.ID.String()
	if err := a.messageStore.ApplyMessagePin(chatJID, sender, messageID, pin, PinExpirySeconds); err != nil {
		log.Println("SetMessagePinned: failed to persist:", err)
	}
	runtime.EventsEmit(a.ctx, "wa:pinned_update", map[string]any{"chatId": chatJID})
	return nil
}

// sendAndStoreLocal sends a prebuilt message and records it locally so the
// UI shows it immediately, mirroring SendMessage's echo path.
func (a *Api) sendAndStoreLocal(chat types.JID, msgContent *waE2E.Message, preview string) (string, error) {
	resp, err := a.waClient.SendMessage(a.ctx, chat, msgContent)
	if err != nil {
		return "", err
	}
	msgEvent := &events.Message{
		Info: types.MessageInfo{
			ID:        resp.ID,
			Timestamp: resp.Timestamp,
			MessageSource: types.MessageSource{
				Chat:     chat,
				IsFromMe: true,
				Sender:   *a.waClient.Store.ID,
			},
		},
		Message: msgContent,
	}
	messageID := a.messageStore.ProcessMessageEvent(a.ctx, a.waClient.Store.LIDs, msgEvent, "")

	var msg any
	if messageID != "" {
		if decodedMsg, derr := a.messageStore.GetDecodedMessage(chat.String(), messageID); derr == nil {
			msg = decodedMsg
		}
	}
	runtime.EventsEmit(a.ctx, "wa:new_message", map[string]any{
		"chatId":      chat.String(),
		"message":     msg,
		"messageText": preview,
		"timestamp":   resp.Timestamp.Unix(),
		"sender":      "You",
		"isFromMe":    true,
	})
	return resp.ID, nil
}

// SendPoll creates a poll in the chat. selectableCount 1 = single answer,
// 0 or len(options) = multiple answers allowed.
func (a *Api) SendPoll(chatJID, name string, options []string, selectableCount int) (string, error) {
	if a.waClient.Store.ID == nil {
		return "", fmt.Errorf("client not logged in")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	clean := make([]string, 0, len(options))
	for _, o := range options {
		if o = strings.TrimSpace(o); o != "" {
			clean = append(clean, o)
		}
	}
	if name == "" || len(clean) < 2 {
		return "", fmt.Errorf("a poll needs a question and at least two options")
	}
	msg := a.waClient.BuildPollCreation(name, clean, selectableCount)
	return a.sendAndStoreLocal(chat, msg, "📊 "+name)
}

// SendShareContact shares a contact card in the chat.
func (a *Api) SendShareContact(chatJID, displayName, phone string) (string, error) {
	if a.waClient.Store.ID == nil {
		return "", fmt.Errorf("client not logged in")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return "", err
	}
	displayName = strings.TrimSpace(displayName)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if displayName == "" || digits == "" {
		return "", fmt.Errorf("contact needs a name and a phone number")
	}
	vcard := fmt.Sprintf(
		"BEGIN:VCARD\nVERSION:3.0\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD",
		displayName, digits, digits)
	msg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: &displayName,
			Vcard:       &vcard,
		},
	}
	return a.sendAndStoreLocal(chat, msg, "👤 "+displayName)
}
