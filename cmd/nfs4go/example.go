package main

import (
	"context"
	"net"
	"os"
	"os/signal"

	"github.com/kuleuven/nfs4go"
	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/vfs"
	"github.com/kuleuven/vfs/fs/nativefs"
	"github.com/kuleuven/vfs/fs/rootfs"
	"github.com/kuleuven/vfs/runas"
	"github.com/sirupsen/logrus"
)

func ExampleLoader(ctx context.Context, conn net.Conn, creds *auth.Creds) (vfs.AdvancedLinkFS, error) {
	fs := rootfs.New(ctx)

	runasContext, err := runas.RunAs(&runas.User{
		UID:    creds.UID,
		GID:    creds.GID,
		Groups: creds.AdditionalGroups,
	})
	if err != nil {
		return nil, err
	}

	err = fs.Mount("/", &nativefs.NativeServerInodeFS{
		NativeFS: &nativefs.NativeFS{
			Root:    "/srv",
			Context: runasContext,
		},
	}, 0)

	return fs, err
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)

	srv, err := nfs4go.Listen(":2050", ExampleLoader)
	if err != nil {
		logrus.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	defer cancel()

	err = srv.Serve(ctx)
	if err != nil {
		logrus.Fatal(err)
	}
}
