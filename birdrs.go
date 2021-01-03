package main

import (
	"flag"
	"fmt"
	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"syscall"
	"unsafe"
)

var release = "dev" // set by build process

var (
	hostKeyFile = flag.String("k", "~/.ssh/id_ed25519", "SSH host key file")
	listenPort  = flag.String("b", ":22", "SSH daemon bind address:port")
	bannerPath  = flag.String("m", "", "path to text file for connection banner")
	verbose     = flag.Bool("v", false, "enable verbose debugging output")
	birdPath    = flag.String("p", "birdc", "path to birdc binary")
)

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage for birdrs (%s) https://github.com/natesales/birdrs:\n", release)
		flag.PrintDefaults()
	}

	flag.Parse()

	if *verbose {
		log.Println("verbose logging enabled")
	}

	var banner []byte
	if *bannerPath != "" { // If bannerPath is set
		_banner, err := ioutil.ReadFile(*bannerPath)
		if err != nil {
			log.Fatalf("reading banner file %s: %v", *bannerPath, err)
		}
		banner = _banner // TODO: Fix scope here
	}

	log.Printf("starting birdrs %s\n", release)
	log.Printf("using bird path: %s\n", *birdPath)

	ssh.Handle(func(s ssh.Session) {
		if *bannerPath != "" { // If bannerPath is set
			s.Write(banner)
		}

		cmd := exec.Command(*birdPath, "-r") // -r flag for restricted mode
		ptyReq, winCh, isPty := s.Pty()
		if isPty {
			cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
			f, err := pty.Start(cmd)
			if err != nil {
				panic(err)
			}
			go func() {
				for win := range winCh {
					syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(win.Height), uint16(win.Width), 0, 0})))
				}
			}()
			go func() { // goroutine to handle
				_, err = io.Copy(f, s) // stdin
				if err != nil {
					if *verbose {
						log.Printf("command f->s copy error: %v\n", err)
					}
				}
			}()
			_, err = io.Copy(s, f) // stdout
			if err != nil {
				if *verbose {
					log.Printf("command s->f copy error: %v\n", err)
				}
			}

			err = cmd.Wait()
			if err != nil {
				if *verbose {
					log.Printf("command wait error: %v\n", err)
				}
			}
		} else {
			io.WriteString(s, "No PTY requested.\n")
			s.Exit(1)
		}
	})

	log.Printf("starting birdrs ssh server on port %s\n", *listenPort)
	log.Fatal(ssh.ListenAndServe(*listenPort, nil, ssh.HostKeyFile(*hostKeyFile)))
}
