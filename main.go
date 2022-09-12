package main

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"k8s.io/client-go/kubernetes"

	"rorycrispin.co.uk/kubefs/resources"
)

var cli *kubernetes.Clientset

func main() {

	mntDir, err := ioutil.TempDir("", "xoyo")
	mntDir = "/tmp/kubefs"
	if err != nil {
		panic(fmt.Errorf("Failed to mount | %w", err))
	}

	root := resources.NewRootContextNode()
	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: true,
			AllowOther: false,
		},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted on %s", mntDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", mntDir)

	// Wait until unmount before exiting
	server.Wait()
}
