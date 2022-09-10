package main

import (
	"fmt"
	"io/ioutil"
	// "flag"
	"log"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"k8s.io/client-go/kubernetes"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
	"rorycrispin.co.uk/kubefs/resources"
)

var cli *kubernetes.Clientset



func main() {
	// clix := getK8sDiscoveryClient()
	// getApiResources(clix)
	//
	contexts, err := kube.GetK8sContexts()
	if err != nil {
		panic(err)
	}
	fmt.Printf("%v", contexts)
	// panic("exit")				//

	// kCtx := flag.String("context", "", "The name of the kubeconfig context to use")
	// flag.Parse()


	// genCli := getK8sUnstructuredClient()
	// getResourcesGeneric(genCli)

	// cli, err = kube.GetK8sClient(*kCtx)
	// if err != nil

	mntDir, err := ioutil.TempDir("", "xoyo")
	mntDir = "/tmp/kubefs"

	if err != nil {
		panic(err)
	}
	fmt.Printf("\nMOUNT AT : %v\n", mntDir)

	root := &resources.RootContextNode{}

	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: true,
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
