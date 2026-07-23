package internal

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gitlab.com/LUwUcifer/bencoder"
)

func (tor *Torrent) ExtractTorrentInfo(path, dest string) error {
	// To implement: check if already exists

	path, dest = filepath.Clean(path), filepath.Clean(dest)
	var _, torrentName string = filepath.Split(path)

	fileExt := filepath.Ext(torrentName)
	if fileExt != ".torrent" {
		return errors.New("file extension not supported")
	}

	torrentName = strings.TrimSuffix(torrentName, fileExt)
	if torrentName == "" {
		torrentName = fmt.Sprintf("Torrent_%d", rand.Int63())
	}

	torrentFileReader, err := os.Open(path)
	if err != nil {
		return err
	}
	decoder := bencoder.NewDecoder(torrentFileReader)

	metaInfoMap, err := decoder.Decode()
	if err != nil {
		return err
	}

	tor.torrPath = path
	tor.destPath = dest
	tor.localData.infoDict.fileName = torrentName

	err = tor.unmarshalBencode(&metaInfoMap)
	if err != nil {
		return err
	}
	return err
}

// parses all local data in torrent files
// returns an error only if it is blocking, otherwise logs the error
func (tor *Torrent) unmarshalBencode(metaInfoMap *bencoder.BencodeDicts) error {
	var err error

	//use announce-list if trackers responding, otherwise use announce
	//https://bittorrent.org/beps/bep_0012.html
	if announceListsListCheck, ok := (*metaInfoMap)["announceList"]; ok {
		announceListsList, ok := announceListsListCheck.(bencoder.BencodeLists)
		if !ok {
		}
		for _, announceList := range announceListsList {
			announceListURLs, ok := announceList.(bencoder.BencodeLists)
			if !ok {
				return MetaInfoInvalidValueType{field: "announce list", wantType: "[]string", haveType: reflect.TypeOf(announceListURLs).String()}
			}
			var announceListURLTier []string
			for _, announceListURL := range announceListURLs {
				announceURL, ok := announceListURL.(bencoder.BencodeBytes)
				if !ok {
					return MetaInfoInvalidValueType{field: "announce list", wantType: "string", haveType: reflect.TypeOf(announceURL).String()}
				}
				announceListURLTier = append(announceListURLTier, string(announceURL))
			}
			tor.localData.trackers.announceList = append(tor.localData.trackers.announceList, announceListURLTier)
		}
	}

	if announceURLCheck, ok := (*metaInfoMap)["announce"]; ok {
		announceURL, ok := announceURLCheck.(bencoder.BencodeBytes)
		if !ok {
		}
		tor.localData.trackers.announce = string(announceURL)
	} else {
		return MetaInfoMissingField{Field: "announce"}
	}

	commentCheck, ok := (*metaInfoMap)["comment"]
	if !ok {
	}
	comment, ok := commentCheck.(bencoder.BencodeBytes)
	if !ok {
		//todo: implement non-blocking custom error logging
		//https://rollbar.com/blog/golang-error-logging-guide/
	}
	tor.localData.metaInfo.comment = string(comment)

	createdByCheck, ok := (*metaInfoMap)["created by"]
	if !ok {
	}
	createdBy, ok := createdByCheck.(bencoder.BencodeBytes)
	if !ok {
	}
	tor.localData.metaInfo.createdBy = string(createdBy)

	encodingCheck, ok := (*metaInfoMap)["encoding"]
	if !ok {
	}
	encoding, ok := encodingCheck.(bencoder.BencodeBytes)
	if !ok {
	}
	tor.localData.metaInfo.encoding = string(encoding)

	creationDateCheck, ok := (*metaInfoMap)["creation date"]
	if !ok {
	}
	creationDate, ok := creationDateCheck.(bencoder.BencodeInts)
	if !ok {
	}
	tor.localData.metaInfo.creationDate = int64(creationDate)

	torrentInfoDictCheck, ok := (*metaInfoMap)["info"]
	if !ok {
	}
	torrentInfoDict, ok := torrentInfoDictCheck.(bencoder.BencodeDicts)
	if !ok {

	}

	//Info Dict Parsing
	pieceLengthCheck, ok := torrentInfoDict["piece length"]
	if !ok {
	}
	pieceLength, ok := pieceLengthCheck.(bencoder.BencodeInts)
	if !ok {
	}
	tor.localData.infoDict.pieceLength = uint64(pieceLength)

	privateCheck, okCheck := torrentInfoDict["private"]
	private, ok := privateCheck.(bencoder.BencodeInts)
	if !ok {
	}
	if !okCheck || private != 1 {
		tor.localData.infoDict.private = false
	} else {
		tor.localData.infoDict.private = true
	}

	err = tor.getFileInfo(&torrentInfoDict)
	if err != nil {
		return err
	}

	//todo: implement validateFiles, getInfoHash, getPieceHashes

	return nil
}

// populates infoDict of torrent file struct
func (tor *Torrent) getFileInfo(torrentInfoDict *bencoder.BencodeDicts) error {
	fileNameCheck, ok := (*torrentInfoDict)["name"]
	if !ok {
	}
	fileName, ok := fileNameCheck.(bencoder.BencodeBytes)
	if !ok {

	}
	tor.localData.infoDict.fileName = string(fileName)

	multipleFilesList, ok := (*torrentInfoDict)["files"]
	filesInfo := multipleFilesList.(bencoder.BencodeLists)
	if ok {
		tor.localData.infoDict.multipleFiles = true
		for _, file := range filesInfo {
			tor.localData.infoDict.fileSpecs = append(tor.localData.infoDict.fileSpecs,
				parseFiles(file, true))
		}
	} else {
		tor.localData.infoDict.multipleFiles = false
		tor.localData.infoDict.fileSpecs = append(tor.localData.infoDict.fileSpecs,
			parseFiles(*torrentInfoDict, false))
	}

	return nil
}

func parseFiles(files bencoder.BencodingTypes, multipleFiles bool) fileSpecs {
	var fileSpecs fileSpecs
	fileDict, ok := files.(bencoder.BencodeDicts)
	if !ok {
	}

	fileSpecsLengthCheck, ok := fileDict["length"]
	if !ok {
	}
	fileSpecsLength, ok := fileSpecsLengthCheck.(bencoder.BencodeInts)
	fileSpecs.length = int64(fileSpecsLength)

	md5Sum, ok := fileDict["md5Sum"]
	if ok {
		md5SumStr, ok := md5Sum.(bencoder.BencodeBytes)
		if !ok {
		}
		fileSpecs.md5sum = [32]byte(md5SumStr)
	}
	if !ok {
	}
	if multipleFiles {
		filePathsCheck, ok := fileDict["path"]
		if !ok {
		}
		filePaths, ok := filePathsCheck.(bencoder.BencodeLists)
		if !ok {
		}
		var filePath []string
		for _, filePathStr := range filePaths {
			filePathStrCheck, ok := filePathStr.(bencoder.BencodeBytes)
			if !ok {
			}
			filePath = append(filePath, string(filePathStrCheck))
		}
		fileSpecs.path = filePath
	}
	return fileSpecs
}
