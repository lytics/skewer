package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func main() {
	rawhosts := ""
	flag.StringVar(&rawhosts, "hosts", "", "comma separated list of hostnames")
	username := os.Getenv("USER")
	flag.StringVar(&username, "user", username, "username")
	rawdur := "1m"
	flag.StringVar(&rawdur, "sleep", rawdur, "duration to sleep between runs")
	alert := ""
	flag.StringVar(&alert, "alert", alert, "if set, command to run when skew is encountered; $MAXSKEW will be set to the max skew")

	flag.Parse()

	hosts := strings.Split(rawhosts, ",")
	if len(hosts) == 0 {
		log.Fatalf("No hosts: %q", rawhosts)
	}
	sort.Strings(hosts)
	for i, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			log.Fatalf("Empty host in list: %q", rawhosts)
		}
		hosts[i] = h
	}

	if username == "" {
		log.Fatal("Empty username")
	}

	dur, err := time.ParseDuration(rawdur)
	if err != nil {
		log.Fatalf("Invalid duration %q: %v", rawdur, err)
	}

	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatalf("Unable to connect to ssh agent on $SSH_AUTH_SOCK (%q): %v", os.Getenv("SSH_AUTH_SOCK"), err)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)},
	}

	servers := make(map[string]*ssh.Client, len(hosts))

	for _, hn := range hosts {
		conn, err := ssh.Dial("tcp", hn+":22", config)
		if err != nil {
			log.Fatalf("Error connecting to %q: %v", hn, err)
		}
		servers[hn] = conn
	}

	for {
		wg := &sync.WaitGroup{}
		rawtimes := make(chan hosttime, len(hosts))
		start := time.Now()
		for _, h := range hosts {
			wg.Add(1)
			go func(hn string) {
				defer wg.Done()
				c := servers[hn]
				sess, err := c.NewSession()
				if err != nil {
					log.Fatalf("Error creating session on host %q: %v", h, err)
				}
				out, err := sess.Output("/bin/date +%s")
				if err != nil {
					log.Fatalf("Error running date on host %q: %v", hn, err)
				}
				sess.Close()
				rawtimes <- hosttime{hn, out}
			}(h)
		}
		wg.Wait()
		elapsed := time.Since(start)

		close(rawtimes)
		min := time.Now().AddDate(1, 1, 1).Unix()
		max := int64(0)
		times := make(map[string]int64, len(hosts))
		for rt := range rawtimes {
			strtime := strings.TrimSpace(string(rt.rawtime))
			t, err := strconv.ParseInt(strtime, 10, 64)
			if err != nil {
				log.Fatalf("Unable to parse time %q: %v", string(rt.rawtime), err)
			}
			if t < min {
				min = t
			}
			if t > max {
				max = t
			}
			times[rt.name] = t
		}

		// If the actual max skew is less than the elapsed time, it may not be
		// actual skew: it may just be due to the commands not running exactly at
		// the same time
		if min-max < (int64(elapsed/time.Second) + 1) {
			goto sleep
		}

		log.Printf("Max expected skew: %d (%s)", int64(elapsed/time.Second), elapsed)
		log.Printf("Actual max skew: %d (%s)", max-min, time.Duration(max-min)*time.Second)
		log.Printf("%10s %16s %3s %3s skew", "host", "time", "min", "max")
		for _, hn := range hosts {
			log.Printf("%10s %16d %3d %3d", hn, times[hn], times[hn]-min, max-times[hn])
		}

		if alert != "" {
			// Run alert command!
			cmd := exec.Command(alert)
			cmd.Env = []string{fmt.Sprintf("MAXSKEW=%d", max)}
			out, err := cmd.CombinedOutput()
			if err != nil {
				log.Fatalf("Alert command failed!\nCommand: %q\nOutput:\n%s", alert, string(out))
			}
		}

	sleep:
		time.Sleep(dur)
	}
}

type hosttime struct {
	name    string
	rawtime []byte
}
