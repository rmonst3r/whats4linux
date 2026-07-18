package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lugvitc/whats4linux/internal/cache"
	"github.com/lugvitc/whats4linux/internal/misc"
	messageStore "github.com/lugvitc/whats4linux/internal/store"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func newMediaTestAPI(t *testing.T) *Api {
	t.Helper()
	oldConfigDir := misc.ConfigDir
	misc.ConfigDir = t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(func() { misc.ConfigDir = oldConfigDir })

	store, err := messageStore.NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	imageCache, err := cache.NewImageCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = imageCache.Close() })

	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("456@s.whatsapp.net")
	info := types.MessageInfo{
		ID:        "text-only",
		Timestamp: time.Now(),
		MessageSource: types.MessageSource{
			Chat: chat, Sender: sender,
		},
	}
	if err := store.InsertMessage(
		&info,
		&waE2E.Message{Conversation: proto.String("hello")},
		"hello",
	); err != nil {
		t.Fatal(err)
	}

	return &Api{
		ctx:          context.Background(),
		messageStore: store,
		imageCache:   imageCache,
	}
}

func TestMediaMethodsRejectMessagesWithoutMedia(t *testing.T) {
	a := newMediaTestAPI(t)

	if _, err := a.GetCachedImage("text-only"); err == nil || !strings.Contains(err.Error(), "no downloadable image") {
		t.Fatalf("GetCachedImage error = %v, want no downloadable image", err)
	}
	if _, err := a.DownloadMedia("123@s.whatsapp.net", "text-only"); err == nil || !strings.Contains(err.Error(), "no downloadable media") {
		t.Fatalf("DownloadMedia error = %v, want no downloadable media", err)
	}
	if _, _, _, _, err := a.downloadMedia(nil); err == nil {
		t.Fatal("downloadMedia(nil) returned no error")
	}
}

func TestMediaMethodsHandleUninitialisedAPI(t *testing.T) {
	a := &Api{}

	if a.GetLinkPreview("message") != nil {
		t.Fatal("GetLinkPreview returned a preview without a message store")
	}
	if a.GetLinkPreviewImage("message") != "" {
		t.Fatal("GetLinkPreviewImage returned data without a message store")
	}
	if a.GetVideoThumbnail("message") != "" {
		t.Fatal("GetVideoThumbnail returned data without a message store")
	}
	if _, err := a.DownloadMedia("chat", "message"); err == nil {
		t.Fatal("DownloadMedia returned no error without a message store")
	}
	if _, err := a.GetCachedImage("message"); err == nil {
		t.Fatal("GetCachedImage returned no error without an image cache")
	}
	if _, err := a.GetCachedAvatar("123@s.whatsapp.net", false); err == nil {
		t.Fatal("GetCachedAvatar returned no error without an image cache")
	}
}
