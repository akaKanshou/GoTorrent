package internal

type announcers struct {
	// Tiered-announce list.
	// In case of only an 'announce' key, announceList has one tier containing that one announce.
	// The reasoning behind this is that further difference in implementations between the two keys can be avoided
	// this way
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

type hashBytes = [20]byte

type infoDict struct {
	pieceLength uint64
	pieces      []hashBytes //SHA1 hashes of every piece
	private     bool

	//Different for single-file/multiple-file system
	fileName      string
	fileSpecs     []fileSpecs
	multipleFiles bool
}
type bencodedData struct {
	sizeData torrentSizeData
	trackers announcers
	metaInfo metaInfo
	infoDict infoDict
	infoHash hashBytes
}
