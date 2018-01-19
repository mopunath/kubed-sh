package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/chzyer/readline"
)

var (
	releaseVersion string
	debugmode      bool
	noprepull      bool
	completer      = readline.NewPrefixCompleter(
		readline.PcItem("cat"),
		readline.PcItem("curl"),
		readline.PcItem("contexts"),
		readline.PcItem("echo"),
		readline.PcItem("env"),
		readline.PcItem("exit"),
		readline.PcItem("help"),
		readline.PcItem("kill"),
		readline.PcItem("literally"),
		readline.PcItem("ls"),
		readline.PcItem("ps"),
		readline.PcItem("pwd"),
		readline.PcItem("use"),
	)
)

func init() {
	if envd := os.Getenv("DEBUG"); envd != "" {
		debugmode = true
	}
	if envp := os.Getenv("KUBEDSH_NOPREPULL"); envp != "" {
		noprepull = true
	}
	// set up the global distributed process table:
	dpt = &DProcTable{
		mux: new(sync.Mutex),
		lt:  make(map[string]DProc),
	}
	err := dpt.BuildDPT()
	if err != nil {
		output(err.Error())
	}
	// set up the environment variables table:
	evt = &EnvVarTable{
		mux: new(sync.Mutex),
		et:  make(map[string]string),
	}
	// load and/or set default environment variables:
	evt.init()
}

func main() {
	kubecontext, err := preflight()
	if err != nil {
		panic(err)
	}
	rl, err := readline.NewEx(&readline.Config{
		AutoComplete:    completer,
		HistoryFile:     "/tmp/readline.tmp",
		InterruptPrompt: "^C",
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = rl.Close()
	}()
	setprompt(rl, kubecontext)
	log.SetOutput(rl.Stderr())
	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "contexts"):
			hcontexts()
		case strings.HasPrefix(line, "curl"):
			hcurl(line)
		case strings.HasPrefix(line, "echo"):
			hecho(line)
		case strings.HasPrefix(line, "env"):
			henv()
		case strings.HasPrefix(line, "help"):
			husage(line)
		case strings.HasPrefix(line, "kill"):
			hkill(line)
		case strings.HasPrefix(line, "literally") || strings.HasPrefix(line, "`"):
			if strings.HasPrefix(line, "`") {
				line = fmt.Sprintf("literally %s", strings.TrimPrefix(line, "`"))
			}
			hliterally(line)
		case strings.HasPrefix(line, "cat"):
			hlocalexec(line)
		case strings.HasPrefix(line, "ls"):
			hlocalexec(line)
		case strings.HasPrefix(line, "ps"):
			hps(line)
		case strings.HasPrefix(line, "pwd"):
			hlocalexec(line)
		case strings.HasPrefix(line, "use"):
			huse(line, rl)
		case line == "exit":
			goto exit
		case line == "version":
			output(releaseVersion)
		case strings.Contains(line, "="):
			envar := strings.Split(line, "=")[0]
			value := strings.Split(line, "=")[1]
			evt.set(envar, value)
		case line == "":
		default:
			hlaunch(line)
		}
	}
exit:
}
