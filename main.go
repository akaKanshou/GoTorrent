package main

import (
	"flag"
	"fmt"
	"os"

	"gitlab.com/akaKanshou/gotorrent/internal"
	"gitlab.com/akaKanshou/gotorrent/logger"
)

func main() {
	var srcPath, destPath string
	var verbose, seedTorrent, freshStart bool

	if len(os.Args) < 2 {
		fmt.Println("Usage: gotorrent <torrent file> [options]")
		logger.Logger.Fatal("torrent file is required as an argument")
		return
	}

	flagSet := flag.NewFlagSet("torrent options", flag.ExitOnError)

	srcPath = os.Args[1]
	flagSet.StringVar(&destPath, "dest", "./", "path to destination")
	flagSet.BoolVar(&verbose, "verbose", false, "enable verbose mode")
	flagSet.BoolVar(&seedTorrent, "seed", false, "enable torrent seeding after completion")
	flagSet.BoolVar(&freshStart, "fresh", false, "ignore previous progress and start from a fresh download")

	flagSet.Parse(os.Args[2:])

	torrentPlayer := internal.NewTorrentPlayer()
	err := torrentPlayer.AddTorrent(srcPath, destPath, verbose, seedTorrent, freshStart)
	if err != nil {
		logger.Logger.Fatal(err.Error())
	}
	logger.Logger.Info("Init files")

	logger.Logger.Info("Starting torrent")
	err = torrentPlayer.StartTorrent()
	if err != nil {
		logger.Logger.Fatal(err.Error())
	}
	logger.Logger.Info("Stopped torrent")
}
