package main

import (
	"flag"
	"fmt"
	"os"

	"gitlab.com/akaKanshou/gotorrent/internal"
	"gitlab.com/akaKanshou/gotorrent/logger"
)

const (
	torrentMulti        = `./torrents/DepressedTrentPatterson-587c7da13d4a35db6f5ab4e3378b8a7f9c3f3f74.torrent`
	torrentSingle       = `./torrents/IntroductiontotheSpecialIssueontheFusionofDomainKnowledgewithDataforDecisionSupport.pdf-58d212578d6b6432a113f6dd3ef0b1a3c528cf9f.torrent`
	buckyTorrent        = `./torrents/big-buck-bunny.torrent`
	nearOptimalTorrent  = `./torrents/Near-OptimalSensorPlacementsinGaussianProcesses_Theory,EfficientAlgorithmsandEmpiricalStudies.pdf-87f561d3d72af9b20f9d0298efddb88f948591db.torrent`
	introductionTorrent = `./torrents/IntroductiontotheSpecialTopiconGrammarInduction,RepresentationofLanguageandLanguageLearning.pdf-71e1c9692c556e252aa7e2f4715c419ee447039b.torrent`
	bigTorrent          = "./torrents/Knowledge-BasedKernelApproximation.pdf-501e017a78ab24b0063bd041f675af7675689b20.torrent"
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
