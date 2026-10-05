package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type torrentProgress struct {
	sizeData      torrentSizeData
	peers         uint64
	downloadSpeed uint64
	isDownloading bool

	err error
}

type modelTUI struct {
	progress     torrentProgress
	progressChan chan torrentProgress

	total         uint64
	torrentPlayer *TorrentPlayer
}

func initialModel(sizeData torrentSizeData, total uint64, isDownloading bool, torrentPlayer *TorrentPlayer) *modelTUI {
	return &modelTUI{
		torrentPlayer: torrentPlayer,
		total:         total,

		progressChan: make(chan torrentProgress, 64),

		progress: torrentProgress{
			sizeData:      sizeData,
			peers:         0,
			downloadSpeed: 0,
			isDownloading: isDownloading,
		},
	}
}

func listenForUpdates(progressChan chan torrentProgress) tea.Cmd {
	return func() tea.Msg {
		progressUpdate, ok := <-progressChan
		if !ok {
			return tea.Quit()
		}

		return progressUpdate
	}
}

func (m *modelTUI) Init() tea.Cmd {
	return listenForUpdates(m.progressChan)
}

func (m *modelTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.torrentPlayer.eventChan <- &stopTorrentEvent{nil}
			return m, tea.Quit
		}
		break

	case tea.InterruptMsg:
		m.torrentPlayer.eventChan <- &stopTorrentEvent{nil}
		return m, tea.Quit

	case torrentProgress:
		m.progress = msg
		return m, listenForUpdates(m.progressChan)
	}

	return m, nil
}

func (m *modelTUI) View() string {
	var s string
	if m.progress.isDownloading {
		s = "Downloading..."
	} else {
		s = "Seeding..."
	}

	data := [][]string{
		{
			strconv.FormatUint(m.total, 10),
			strconv.FormatUint(m.total-m.progress.sizeData.Left, 10),
			strconv.FormatUint(m.progress.sizeData.Left, 10),

			strconv.FormatUint(m.progress.peers, 10),

			strconv.FormatUint(m.progress.sizeData.Downloaded, 10),
			strconv.FormatUint(m.progress.sizeData.Uploaded, 10),
		},
	}

	progressTable := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		Rows(data...).
		StyleFunc(func(row, col int) lipgloss.Style { return lipgloss.NewStyle().Padding(0, 2) })

	progressBar := ""
	if m.progress.isDownloading {
		progressPercentage := ((m.total - m.progress.sizeData.Left) * 100) / m.total
		eta := "∞"
		if m.progress.downloadSpeed > 0 {
			eta = (time.Duration((m.total-m.progress.sizeData.Left)/m.progress.downloadSpeed) * time.Second).String()
		}
		progressBar = fmt.Sprintf("Progress (%d%%): [%s%s]\nETA:%s\n",
			progressPercentage, strings.Repeat("=", int(progressPercentage/2)), strings.Repeat(" ", 50-int(progressPercentage/2)), eta)
	}

	s = fmt.Sprintf("%s\n%s\n%s", s, progressTable.Render(), progressBar)

	if m.progress.err != nil {
		s = fmt.Sprintf("%s\nAn error occurred!\n%s\n", s, m.progress.err.Error())
	}

	return s
}

var (
	headers = []string{"Total", "Downloaded", "Left", "Peers", "Bytes downloaded", "Bytes uploaded"}
)
