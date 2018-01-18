package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	prePullImgDS = `apiVersion: APIVERSION
kind: DaemonSet
metadata:
  name: PREPULLID
  annotations:
    source: "https://gist.github.com/itaysk/7bc3e56d69c4d72a549286d98fd557dd"
  labels:
    gen: kubed-sh
    scope: pre-flight
spec:
  selector:
    matchLabels:
      name: prepull
  template:
    metadata:
      labels:
        name: prepull
    spec:
      initContainers:
      - name: prepull
        image: docker
        command: ["docker", "pull", "IMG"]
        volumeMounts:
        - name: docker
          mountPath: /var/run
      volumes:
      - name: docker
        hostPath:
          path: /var/run
      containers:
      - name: pause
        image: gcr.io/google_containers/pause
`
)

// output prints primary, output messages to shell
func output(msg string) {
	fmt.Println(msg)
}

// info prints secondary, non-output info to shell
func info(msg string) {
	fmt.Printf("\033[34m%s\033[0m\n", msg)
}

// warn prints warning messages to shell
func warn(msg string) {
	fmt.Printf("\033[31m%s\033[0m\n", msg)
}

// debug prints debug messages to shell
func debug(msg string) {
	if debugmode {
		fmt.Printf("\033[33m%s\033[0m\n", msg)
	}
}

func preflight() (string, error) {
	checkruntime()
	cversion, sversion, err := whatversion()
	if err != nil {
		return "", err
	}
	info(fmt.Sprintf("Detected Kubernetes client in version %s and server in version %s\n", cversion, sversion))
	prepullimgs(sversion)
	kubecontext, err := kubectl("config", "current-context")
	if err != nil {
		return "", err
	}
	return kubecontext, nil
}

func checkruntime() {
	switch runtime.GOOS {
	case "linux":
		fmt.Printf("Note: As you're running kubed-sh on Linux you can directly launch binaries.\n\n")
	default:
		fmt.Printf("Note: It seems you're running kubed-sh in a non-Linux environment (detected: %s),\nso make sure the binaries you launch are Linux binaries in ELF format.\n\n", runtime.GOOS)
	}
}

func whatversion() (string, string, error) {
	res, err := kubectl("version", "--short")
	if err != nil {
		return "", "", err
	}
	// the following is something like 'Client Version: v1.9.1':
	clientv := strings.Split(res, "\n")[0]
	clientv = strings.Split(clientv, " ")[2]
	// the following is something like 'Server Version: v1.7.2':
	serverv := strings.Split(res, "\n")[1]
	serverv = strings.Split(serverv, " ")[2]
	return clientv, serverv, nil
}

func prepullimgs(serverversion string) {
	if noprepull { // user told us not to pre-pull images
		return
	}
	ppdaemonsets, _ := kubectl("get", "daemonset",
		"--selector=gen=kubed-sh,scope=pre-flight",
		"-o=custom-columns=:metadata.name", "--no-headers")
	if ppdaemonsets != "" { // the Daemonset is already active
		return
	}
	img := evt.get("BINARY_IMAGE")
	err := prepullimg(serverversion, "prepullbin", img, "/tmp/kubed-sh_ds_binary.yaml")
	if err != nil {
		info("Wasn't able to pre-pull container image " + img)
	}
	img = evt.get("NODE_IMAGE")
	err = prepullimg(serverversion, "prepulljs", img, "/tmp/kubed-sh_ds_node.yaml")
	if err != nil {
		info("Wasn't able to pre-pull container image " + img)
	}
	img = evt.get("PYTHON_IMAGE")
	err = prepullimg(serverversion, "prepullpy", img, "/tmp/kubed-sh_ds_python.yaml")
	if err != nil {
		info("Wasn't able to pre-pull container image " + img)
	}
	img = evt.get("RUBY_IMAGE")
	err = prepullimg(serverversion, "prepullrb", img, "/tmp/kubed-sh_ds_ruby.yaml")
	if err != nil {
		info("Wasn't able to pre-pull container image " + img)
	}
	output("Pre-pulling images, this may take up to 30 seconds to complete, please stand by.\nDon't worry, this is a one-time only operation ;)")
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for t := range ticker.C {
			_ = t
			fmt.Printf(".")
		}
	}()
	time.Sleep(30 * time.Second)
	ticker.Stop()
}

func prepullimg(serverversion, targetid, targetimg, targetmanifest string) error {
	// based on https://codefresh.io/blog/single-use-daemonset-pattern-pre-pulling-images-kubernetes/
	var ds string
	switch {
	case strings.HasPrefix(serverversion, "v1.5") || strings.HasPrefix(serverversion, "v1.6") || strings.HasPrefix(serverversion, "v1.7"):
		ds = strings.Replace(prePullImgDS, "APIVERSION", "extensions/v1beta1", -1)
	default:
		ds = strings.Replace(prePullImgDS, "APIVERSION", "apps/v1beta2", -1)
	}
	ds = strings.Replace(ds, "IMG", targetimg, -1)
	ds = strings.Replace(ds, "PREPULLID", targetid, -1)
	err := ioutil.WriteFile(targetmanifest, []byte(ds), 0644)
	if err != nil {
		return err
	}
	res, err := kubectl("create", "-f", targetmanifest)
	if err != nil {
		return err
	}
	debug(res)
	return nil
}

func shellout(cmd string, args ...string) (string, error) {
	result := ""
	var out bytes.Buffer
	log.Debug(cmd, args)
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	c.Stderr = os.Stderr
	c.Stdout = &out
	err := c.Run()
	if err != nil {
		return result, err
	}
	result = strings.TrimSpace(out.String())
	return result, nil
}

func shelloutbg(cmd string, args ...string) error {
	log.Debug(cmd, args)
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	err := c.Run()
	if err != nil {
		return err
	}
	return nil
}

func kubectl(cmd string, args ...string) (string, error) {
	kubectlbin, err := shellout("which", "kubectl")
	if err != nil {
		return "", err
	}
	all := append([]string{cmd}, args...)
	result, err := shellout(kubectlbin, all...)
	if err != nil {
		return "", err
	}
	return result, nil
}

func kubectlbg(cmd string, args ...string) error {
	kubectlbin, err := shellout("which", "kubectl")
	if err != nil {
		return err
	}
	all := append([]string{cmd}, args...)
	err = shelloutbg(kubectlbin, all...)
	if err != nil {
		return err
	}
	return nil
}
