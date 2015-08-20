package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"os/user"
	"runtime"

	"github.com/omakoto/yt-up/oauth"
)

var (
	clientId     = flag.String("clientid", "", "Client ID")
	clientSecret = flag.String("secret", "", "Client secret")
)

// openURL opens a browser window to the specified location.
// This code originally appeared at:
//   http://stackoverflow.com/questions/10377243/how-can-i-launch-a-process-that-is-not-a-file-in-go
func openURL(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", "http://localhost:4001/").Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("Cannot open URL %s on this platform", url)
	}
	return err
}

func getHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr.HomeDir
}

func buildConfig(scope string) (*oauth.Config, error) {
	if *clientId == "" {
		log.Fatalf("You must provide an oauth client ID with -clientid")
	}

	if *clientSecret == "" {
		log.Fatalf("You must provide an oauth client secret with -secret")
	}

	return &oauth.Config{
		ClientId:       *clientId,
		ClientSecret:   *clientSecret,
		Scope:          scope,
		AuthURL:        "https://accounts.google.com/o/oauth2/auth",
		TokenURL:       "https://accounts.google.com/o/oauth2/token",
		RedirectURL:    "http://localhost:8080/",
		TokenCache:     oauth.CacheFile(getHomeDir() + "/.yt-up.oauth.cache"),
		AccessType:     "offline",
		ApprovalPrompt: "force",
	}, nil
}

// startWebServer starts a web server that listens on http://localhost:8080.
// The webserver waits for an oauth code in the three-legged auth flow.
func startWebServer() (codeCh chan string, err error) {
	listener, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		return nil, err
	}
	codeCh = make(chan string)
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.FormValue("code")
		codeCh <- code // send code to OAuth flow
		listener.Close()
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Received code: %v\r\nYou can now safely close this browser window.", code)
	}))

	return codeCh, nil
}

// buildOAuthHTTPClient takes the user through the three-legged OAuth flow.
// It opens a browser in the native OS or outputs a URL, then blocks until
// the redirect completes to the /oauth2callback URI.
// It returns an instance of an HTTP client that can be passed to the
// constructor of the YouTube client.
func buildOAuthHTTPClient(scope string) (*http.Client, error) {
	config, err := buildConfig(scope)
	if err != nil {
		msg := fmt.Sprintf("Cannot read configuration file: %v", err)
		return nil, errors.New(msg)
	}

	transport := &oauth.Transport{Config: config}

	// Try to read the token from the cache file.
	// If an error occurs, do the three-legged OAuth flow because
	// the token is invalid or doesn't exist.
	token, err := config.TokenCache.Token()
	if err != nil {
		// Start web server.
		// This is how this program receives the authorization code
		// when the browser redirects.
		codeCh, err := startWebServer()
		if err != nil {
			return nil, err
		}

		// Open url in browser
		url := config.AuthCodeURL("")
		// fmt.Println("URL=" + url)
		err = openURL(url)
		if err != nil {
			log.Println("Visit the URL below to get a code.",
				" This program will pause until the site is visted.")
		} else {
			log.Println("Your browser has been opened to an authorization URL.",
				" This program will resume once authorization has been provided.")
		}
		// fmt.Println(url)

		// Wait for the web server to get the code.
		code := <-codeCh

		// This code caches the authorization code on the local
		// filesystem, if necessary, as long as the TokenCache
		// attribute in the config is set.
		token, err = transport.Exchange(code)
		if err != nil {
			return nil, err
		}
	}

	transport.Token = token
	return transport.Client(), nil
}
