package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/context"

	"github.com/google/google-api-go-client/youtube/v3"
	"github.com/omakoto/bashcomp"
)

var (
	filename    = flag.String("filename", "", "Name of video file to upload")
	title       = flag.String("title", "", "Video title")
	description = flag.String("description", "", "Video description")
	category    = flag.String("category", "", "Video category") // TODO
	keywords    = flag.String("keywords", "", "Comma separated list of video keywords")
	privacy     = flag.String("privacy", "unlisted", "Video privacy status (private|unlisted|public)")
	playlist    = flag.String("playlist", "", "Playlist name to add video to")
	lastPercent = (int64)(0)
)

const (
	SCOPE = "https://www.googleapis.com/auth/youtube.upload"
)

func progress(current, total int64) {
	newPercent := current * 100 / total
	if newPercent > lastPercent {
		msg := fmt.Sprintf("Uploading... (%d KB / %d KB uploaded, %d%%)", current/1024, total/1024, newPercent)
		if terminal.IsTerminal(syscall.Stdout) {
			fmt.Printf("\x1b[K%s\r", msg)
		} else {
			log.Printf("%s\n", msg)
		}
	}
	lastPercent = newPercent
}

func main() {
	flag.Parse()

	bashcomp.HandleBashCompletion()

	if *filename == "" {
		log.Fatalf("Specify a filename of a video file with -filename")
	}

	log.Printf("Requesting auth token...\n")

	client, err := buildOAuthHTTPClient(SCOPE)
	if err != nil {
		log.Fatalf("Error building OAuth client: %v", err)
	}

	log.Printf("Uploading %s...\n", *filename)

	service, err := youtube.New(client)
	if err != nil {
		log.Fatalf("Error creating YouTube client: %v", err)
	}

	if *playlist != "" {
		playlists := youtube.NewPlaylistsService(service)
		playListsCall := playlists.List("snippet")
		playListsCall.Mine(true)
		playlistsResult, err := playListsCall.Do()
		if err != nil {
			log.Fatalf("Error listing playlists: %v", err)
		}

		for item := range playlistsResult.Items {
			log.Printf("%s\n", item)
		}

		return
	}

	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       *title,
			Description: *description,
			CategoryId:  *category,
		},
		Status: &youtube.VideoStatus{PrivacyStatus: *privacy},
	}

	// The API returns a 400 Bad Request response if tags is an empty string.
	if strings.Trim(*keywords, "") != "" {
		upload.Snippet.Tags = strings.Split(*keywords, ",")
	}

	call := service.Videos.Insert("snippet,status", upload)

	call.ProgressUpdater(progress)

	file, err := os.Open(*filename)
	defer file.Close()
	if err != nil {
		log.Fatalf("Error opening %v: %v", *filename, err)
	}
	fi, err := file.Stat()
	if err != nil {
		log.Fatalf("Error obtaining file size %v: %v", *filename, err)
	}

	size := fi.Size()

	call.ResumableMedia(context.TODO(), file, size, "")

	start := time.Now()

	response, err := call.Do()
	if err != nil {
		log.Fatalf("Error making YouTube API call: %v", err)
	}
	end := time.Now()

	duration := end.Sub(start)

	oneHundreadMegMinutes := float64(duration.Minutes() * 100.0 * 1024.0 * 1024.0 / float64(size))
	if terminal.IsTerminal(syscall.Stdout) {
		fmt.Printf("\n")
	}

	log.Printf("Uploaded %.1f MB in %s, %.1f minutes for 100MB : http://youtube.com/watch?v=%v\n", float64(size)/(1024.0*1024.0), duration, oneHundreadMegMinutes, response.Id)
}
