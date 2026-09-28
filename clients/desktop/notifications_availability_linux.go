//go:build linux

package desktop

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

func notificationDaemonReady() bool {
	connection, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var available bool
	err = connection.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, "org.freedesktop.Notifications").Store(&available)
	return err == nil && available
}
