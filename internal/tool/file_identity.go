package tool

import "os"

type fileSnapshotIdentity struct {
	size      int64
	modTimeNS int64
	ctimeNS   int64
	dev       uint64
	ino       uint64
}

func snapshotIdentity(info os.FileInfo) fileSnapshotIdentity {
	id := fileSnapshotIdentity{
		size:      info.Size(),
		modTimeNS: info.ModTime().UnixNano(),
	}
	if dev, ino, ctimeNS, ok := platformFileIdentity(info); ok {
		id.dev = dev
		id.ino = ino
		id.ctimeNS = ctimeNS
	}
	return id
}
