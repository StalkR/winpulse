// +build windows

/*
Binary winpulse captures Audio from Windows and streams it to a PulseAudio server.
It can stream over SSH with pacat.
It shows in systray with PulseAudio icon, right-click to exit.
Build with `-ldflags -H=windowsgui` to avoid launching a console window.
*/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/StalkR/winpulse/cygwin"
	"github.com/StalkR/winpulse/icon"
	"github.com/StalkR/winpulse/winaudio"
	"github.com/getlantern/systray"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	flagSSH      = flag.String("ssh", "", "PulseAudio server to connect to via SSH + pacat (user@host:port).")
	flagPassword = flag.String("password", "", "SSH password, if not provided will try to find a running Cygwin ssh-agent.")
)

var sshRE = regexp.MustCompile(`^([^@]*)@(.*)$`) // matches user@host:port

func main() {
	flag.Parse()
	if *flagSSH == "" {
		flag.PrintDefaults()
		return
	}
	if *flagSSH != "" && !sshRE.MatchString(*flagSSH) {
		log.Fatal("invalid user@host:port")
	}
	ctx := context.Background()
	systray.Run(func() { onReady(ctx) }, onExit)
}

func onReady(ctx context.Context) {
	systray.SetTitle("WinPulse")
	play := systray.AddMenuItem("Play", "Play")
	play.Hide()
	stop := systray.AddMenuItem("Stop", "Stop")
	stop.Hide()
	systray.AddSeparator()
	exit := systray.AddMenuItem("Exit", "Exit")
	go func() {
		<-exit.ClickedCh
		systray.Quit()
	}()
	for {
		ctx, cancel := context.WithCancel(ctx)
		go start(ctx)
		log.Print("Started")
		systray.SetTooltip("Started")
		systray.SetIcon(icon.Color)
		stop.Show()

		<-stop.ClickedCh
		cancel()
		stop.Hide()
		log.Print("Stopped")
		systray.SetTooltip("Stopped")
		systray.SetIcon(icon.Gray)
		play.Show()

		<-play.ClickedCh
		play.Hide()
	}
}

func onExit() {
	log.Print("Exiting")
	os.Exit(0)
}

func start(ctx context.Context) {
	r, w := io.Pipe()
	go func() {
		defer r.Close()
		for ; ; time.Sleep(time.Second) {
			log.Print("Connecting to PulseAudio...")
			if err := play(ctx, r); err != nil && err != io.EOF {
				log.Printf("error: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	defer w.Close()
	for ; ; time.Sleep(time.Second) {
		log.Print("Capturing Windows Audio...")
		if err := winaudio.Capture(ctx, w); err != nil {
			log.Printf("error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

var errStop = errors.New("stop")

func play(ctx context.Context, stream io.Reader) error {
	userHost := sshRE.FindStringSubmatch(*flagSSH)
	if len(userHost) == 0 { // verified earlier anyway
		return fmt.Errorf("invalid user@host:port")
	}
	return playSSH(ctx, stream, userHost[1], userHost[2])
}

func playSSH(ctx context.Context, stream io.Reader, user, host string) error {
	client, err := connectSSH(user, host)
	if err != nil {
		return err
	}
	defer client.Close()
	log.Print("Connected to server")
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = stream
	if err := session.Start("pacat -p --format float32le"); err != nil {
		return err
	}
	log.Print("Connected to PulseAudio")
	systray.SetTooltip("Connected to PulseAudio")
	errc := make(chan error)
	go func() { errc <- session.Wait() }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errc:
		return err
	}
}

func connectSSH(user, host string) (*ssh.Client, error) {
	auth := ssh.Password(*flagPassword)

	if *flagPassword == "" {
		ag, err := cygwin.SSHAgent()
		if err != nil {
			return nil, err
		}
		defer ag.Close()
		auth = ssh.PublicKeysCallback(agent.NewClient(ag).Signers)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	return ssh.Dial("tcp", host, config)
}
