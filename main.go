package main

import (
	"fmt"
	"io/ioutil"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"k8s.io/client-go/kubernetes"
	"go.uber.org/zap"

	"rorycrispin.co.uk/kubefs/resources"
)

var cli *kubernetes.Clientset

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	log := logger.Sugar()

	mntDir, err := ioutil.TempDir("", "xoyo")
	mntDir = "/tmp/kubefs"

	if err != nil {
		panic(fmt.Errorf("Failed to mount | %w", err))
	}

	root := resources.NewRootContextNode(log)
	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: false,
			AllowOther: false,
		},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Infof("Mounted on %s", mntDir)
	log.Infof("Unmount by calling 'fusermount -u %s'", mntDir)

	// Wait until unmount before exiting
	server.Wait()
}
