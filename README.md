kubefs is a FUSE driver for Kubernetes
====

Use the plaintext tools which you already know inside out to interact with your clusters.

kubefs makes your clusters available as a tree of plaintext files. 

`kubectl --context majestic-gnat --namespace flycatcher get po nginx-1 -oyaml`
<br>
becomes
<br>
`cat /tmp/kubefs/majestic-gnat/namespaces/flycatcher/pods/nginx-1/def.yaml`

You can exec commands on containers too;

`kubectl --context majestic-gnat --namespace flycatcher exec nginx-1 --container nginx-ingress -- cat blah`
<br>
becomes<br>
`echo "cat blah" >> /tmp/kubefs/majestic-gnat/namespaces/flycatcher/pods/nginx-1/containers/nginx-ingress`

But it's true power comes when you use your existing tools:

Diff two pod definitions with emacs:<br>
`ediff /tmp/kubefs/majestic-gnat/namespaces/flycatcher/pods/nginx-1/def.yaml /tmp/kubefs/majestic-gnat/namespaces/flycatcher/pods/nginx-2/def.yaml`
