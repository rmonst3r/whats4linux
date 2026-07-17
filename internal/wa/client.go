package wa

import (
	"context"

	"github.com/lugvitc/whats4linux/internal/settings"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func NewClient(ctx context.Context, container *sqlstore.Container) *whatsmeow.Client {
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", settings.GetLogLevel(), true)
	cli := whatsmeow.NewClient(deviceStore, clientLog)
	// Without this, a full app state sync applies patches but dispatches NO
	// events (whatsmeow drops them unless the flag is set), so archive/pin/
	// mute state synced from the phone would never reach our handlers.
	cli.EmitAppStateEventsOnFullSync = true
	return cli
}
