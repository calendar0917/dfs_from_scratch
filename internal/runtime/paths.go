package runtime

const (
	defaultMasterPersistPath = "persist.log"
	defaultVolumeStorageRoot = "./data"
	defaultVolumeNodeID      = "volume"
)

func ResolveMasterPersistPath(path string) string {
	if path == "" {
		return defaultMasterPersistPath
	}
	return path
}

func ResolveVolumeStorageDir(nodeID, dir string) string {
	if dir != "" {
		return dir
	}
	if nodeID == "" {
		nodeID = defaultVolumeNodeID
	}
	return defaultVolumeStorageRoot + "/" + nodeID
}
