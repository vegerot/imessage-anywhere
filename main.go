// iMessage Anywhere: a Proxy on the Host Mac that serves Messages to my tailnet devices.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	imsgPath := flag.String("imsg", "imsg", "path to the imsg executable")
	listen := flag.String("listen", "172.16.1.2:7171,127.0.0.1:7171",
		"comma-separated addresses to listen on: the WireGuard tunnel address and loopback")
	configDir, _ := os.UserConfigDir() // ~/Library/Application Support
	certificate := flag.String("certificate", filepath.Join(configDir, "imessage-anywhere", "certificate.pem"),
		"the HTTPS certificate chain and its private key in one PEM file (scripts/fetch-certificate.sh)")
	flag.Parse()

	cmd := exec.Command(*imsgPath, "rpc")
	// With SSH_CONNECTION set, imsg reads contact names from the AddressBook database
	// under Full Disk Access. Without it, imsg asks for the Contacts permission, and that
	// prompt never appears for a background process (experiment E3).
	cmd.Env = append(os.Environ(), "SSH_CONNECTION=local 0 local 0")
	engine, err := StartEngine(cmd)
	if err != nil {
		log.Fatalf("starting imsg: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = engine.Ready(ctx)
	cancel()
	if err != nil {
		log.Fatal(err) // launchd retries; the grant takes effect on the next start
	}
	handler := NewServer(engine)

	for _, addr := range strings.Split(*listen, ",") {
		// Before the WireGuard tunnel is up, its address doesn't exist and this fails;
		// exiting lets launchd retry until it does.
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("listening on %s: %v", addr, err)
		}
		// Browsers only allow notifications and installing the page over HTTPS or on
		// loopback. So loopback stays plain HTTP (for checks on the Host Mac), and the
		// tunnel address serves HTTPS with the certificate for the Proxy's tailnet name.
		host, _, _ := net.SplitHostPort(addr)
		if netip.MustParseAddr(host).IsLoopback() {
			log.Printf("serving on http://%s", addr)
			go func() { log.Fatal(http.Serve(ln, handler)) }()
			continue
		}
		log.Printf("serving on https://%s", addr)
		// A missing certificate exits here; launchd retries until fetch-certificate.sh has run.
		go func() { log.Fatal((&http.Server{Handler: handler}).ServeTLS(ln, *certificate, *certificate)) }()
	}

	<-engine.Done()
	log.Fatal("imsg rpc exited; exiting so launchd restarts the Proxy")
}
