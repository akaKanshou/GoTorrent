package internal

type announcers struct {
	announce     string
	announceList [][]string
}
type metaInfo struct {
	creationDate int64
	comment      string
	createdBy    string
	encoding     string
}

type fileSpecs struct {
	length int64
	md5sum [32]byte
	path   []string
}
type infoDict struct {
	pieceLength uint64
	pieces      [][]string //SHA1 hashes of every piece
	private     bool

	//Different for single-file/multiple-file system
	fileName      string
	fileSpecs     []fileSpecs
	multipleFiles bool
}
type bencodedData struct {
	trackers announcers
	metaInfo metaInfo
	infoDict infoDict
}
