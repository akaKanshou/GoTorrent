package internal

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"gitlab.com/LUwUcifer/gotorrent/logger"
	"go.uber.org/zap"
)

const (
	pUDP byte = iota
	pHTTP
	pWSS
)

type tracker interface {
	getProtocol() byte
}

func (tr *trackerUDP) getProtocol() byte {
	return pUDP
}

func (tr *trackerHTTP) getProtocol() byte {
	return pHTTP
}

// initTrackerList generates a tracker tier list for the given announce-url list
// (!) Any announce-urls with unknown or unimplemented protocols are silently dropped without any error - might be
// subject to change
func initTrackerList(announceList [][]string) [][]tracker {
	var trackerList [][]tracker
	for _, tier := range announceList {
		rand.Shuffle(len(tier), func(i, j int) {
			tier[i], tier[j] = tier[j], tier[i]
		})

		var trackers []tracker
		for _, url := range tier {
			index := strings.Index(url, "://")
			if index == -1 {
				logger.Logger.Warn("Invalid announce url", zap.String("url", url))
				continue
			}

			switch url[:index] {
			case "udp":
				url = strings.TrimSuffix(strings.TrimPrefix(url, "udp://"), "/announce")
				// TODO: Is this level of nesting truly necessary? This struct needs flattening and some fields are redundant
				trackers = append(trackers, &trackerUDP{
					trackerData: trackerData{
						metaData: metaData{
							announceUrl: url,
						},
					},
				})
				break
			case "http", "https":
				trackers = append(trackers, &trackerHTTP{
					trackerData: trackerData{
						metaData: metaData{
							announceUrl: url,
						},
					},
				})
				break
			default:
				logger.Logger.Warn("Unimplemented or Invalid protocol", zap.String("url", url))
			}

		}

		fmt.Println(trackers)
		if len(trackers) == 0 {
			continue
		}
		trackerList = append(trackerList, trackers)
	}
	logger.Logger.Info("Initialized tracker list", zap.Int("trackerList size", len(trackerList)))
	return trackerList
}
