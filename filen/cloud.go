package filen

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FilenCloudDienste/filen-sdk-go/filen/crypto"
	"github.com/FilenCloudDienste/filen-sdk-go/filen/util"
	"github.com/google/uuid"
	"strings"
	"time"
)

// File represents a file on the cloud drive.
type File struct {
	UUID          string    // the UUID of the cloud item
	Name          string    // the file name
	Size          int64     // the file size in bytes
	MimeType      string    // the MIME type of the file
	EncryptionKey []byte    // the key used to encrypt the file data
	Created       time.Time // when the file was created
	LastModified  time.Time // when the file was last modified
	ParentUUID    string    // the [Directory.UUID] of the file's parent directory
	Favorited     bool      // whether the file is marked a favorite
	Region        string    // the file's storage region
	Bucket        string    // the file's storage bucket
	Chunks        int       // how many 1 MiB chunks the file is partitioned into
}

// Directory represents a directory on the cloud drive.
type Directory struct {
	UUID       string    // the UUID of the cloud item
	Name       string    // the directory name
	ParentUUID string    // the [Directory.UUID] of the directory's parent directory (or zero value for the root directory)
	Color      string    // the color assigned to the directory (zero value means default color)
	Created    time.Time // when the directory was created
	Favorited  bool      // whether the directory is marked a favorite
}

// GetBaseFolderUUID fetches the UUID of the cloud drive's root directory.
func (filen *Filen) GetBaseFolderUUID() (string, error) {
	userBaseFolder, err := filen.client.GetUserBaseFolder()
	if err != nil {
		return "", err
	}
	return userBaseFolder.UUID, nil
}

// FindItemUUID finds a cloud item by its path and returns its UUID.
// Returns an empty string if none was found.
// Use this instead of FindItem to correctly handle paths pointing to the base directory.
func (filen *Filen) FindItemUUID(path string, requireDirectory bool) (string, error) {
	if len(strings.Join(strings.Split(path, "/"), "")) == 0 { // empty path
		baseFolderUUID, err := filen.GetBaseFolderUUID()
		if err != nil {
			return "", err
		}
		return baseFolderUUID, nil
	} else {
		file, directory, err := filen.FindItem(path, requireDirectory)
		if err != nil {
			return "", err
		}
		if file != nil {
			return file.UUID, nil
		}
		if directory != nil {
			return directory.UUID, nil
		}
		return "", nil
	}
}

// FindItem find a cloud item by its path and returns it (either the File or the Directory will be returned).
// Set requireDirectory to differentiate between files and directories with the same path (otherwise, the file will be found).
// Returns nil for both File and Directory if none was found.
func (filen *Filen) FindItem(path string, requireDirectory bool) (*File, *Directory, error) {
	baseFolderUUID, err := filen.GetBaseFolderUUID()
	if err != nil {
		return nil, nil, err
	}

	segments := strings.Split(path, "/")
	if len(strings.Join(segments, "")) == 0 {
		return nil, nil, errors.New(fmt.Sprintf("no segments in path %s", path))
	}

	currentUUID := baseFolderUUID
SegmentsLoop:
	for segmentIdx, segment := range segments {
		if segment == "" {
			continue
		}

		files, directories, err := filen.ReadDirectory(currentUUID)
		if err != nil {
			return nil, nil, err
		}
		if !requireDirectory {
			for _, file := range files {
				if file.Name == segment {
					return file, nil, nil
				}
			}
		}
		for _, directory := range directories {
			if directory.Name == segment {
				if segmentIdx == len(segments)-1 {
					return nil, directory, nil
				} else {
					currentUUID = directory.UUID
					continue SegmentsLoop
				}
			}
		}
		return nil, nil, nil
	}
	return nil, nil, errors.New("unreachable")
}

// FindDirectoryOrCreate finds a cloud directory by its path and returns its UUID.
// If the directory cannot be found, it (and all non-existent parent directories) will be created.
func (filen *Filen) FindDirectoryOrCreate(path string) (string, error) {
	baseFolderUUID, err := filen.GetBaseFolderUUID()
	if err != nil {
		return "", err
	}

	segments := strings.Split(path, "/")
	if len(strings.Join(segments, "")) == 0 {
		return baseFolderUUID, nil
	}

	currentUUID := baseFolderUUID
SegmentsLoop:
	for _, segment := range segments {
		if segment == "" {
			continue
		}

		_, directories, err := filen.ReadDirectory(currentUUID)
		if err != nil {
			return "", err
		}
		for _, directory := range directories {
			if directory.Name == segment {
				// directory found
				currentUUID = directory.UUID
				continue SegmentsLoop
			}
		}
		// create directory
		directory, err := filen.CreateDirectory(currentUUID, segment)
		if err != nil {
			return "", err
		}
		currentUUID = directory.UUID
	}
	return currentUUID, nil
}

// ReadDirectory fetches the files and directories that are children of a directory (specified by UUID).
func (filen *Filen) ReadDirectory(uuid string) ([]*File, []*Directory, error) {
	// fetch directory content
	directoryContent, err := filen.client.GetDirectoryContent(uuid)
	if err != nil {
		return nil, nil, err
	}

	// transform files
	files := make([]*File, 0)
	for _, file := range directoryContent.Uploads {
		metadataStr, err := crypto.DecryptMetadataAllKeys(file.Metadata, filen.MasterKeys)
		if err != nil {
			return nil, nil, err
		}
		var metadata struct {
			Name         string `json:"name"`
			Size         int    `json:"size"`
			MimeType     string `json:"mime"`
			Key          string `json:"key"`
			LastModified int    `json:"lastModified"`
		}
		err = json.Unmarshal([]byte(metadataStr), &metadata)
		if err != nil {
			return nil, nil, err
		}

		files = append(files, &File{
			UUID:          file.UUID,
			Name:          metadata.Name,
			Size:          int64(metadata.Size),
			MimeType:      metadata.MimeType,
			EncryptionKey: []byte(metadata.Key),
			Created:       util.TimestampToTime(int64(file.Timestamp)),
			LastModified:  util.TimestampToTime(int64(metadata.LastModified)),
			ParentUUID:    file.Parent,
			Favorited:     file.Favorited == 1,
			Region:        file.Region,
			Bucket:        file.Bucket,
			Chunks:        file.Chunks,
		})
	}

	// transform directories
	directories := make([]*Directory, 0)
	for _, directory := range directoryContent.Folders {
		nameStr, err := crypto.DecryptMetadataAllKeys(directory.Name, filen.MasterKeys)
		if err != nil {
			return nil, nil, err
		}
		var name struct {
			Name string `json:"name"`
		}
		err = json.Unmarshal([]byte(nameStr), &name)
		if err != nil {
			return nil, nil, err
		}

		directories = append(directories, &Directory{
			UUID:       directory.UUID,
			Name:       name.Name,
			ParentUUID: directory.Parent,
			Color:      "<none>", //TODO tmp
			Created:    util.TimestampToTime(int64(directory.Timestamp)),
			Favorited:  directory.Favorited == 1,
		})
	}

	return files, directories, nil
}

// TrashFile moves a file to trash.
func (filen *Filen) TrashFile(uuid string) error {
	return filen.client.TrashFile(uuid)
}

// CreateDirectory creates a new directory.
func (filen *Filen) CreateDirectory(parentUUID string, name string) (*Directory, error) {
	directoryUUID := uuid.New().String()

	// encrypt metadata
	metadata := struct {
		Name string `json:"name"`
	}{name}
	metadataStr, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	metadataEncrypted, err := crypto.EncryptMetadata(string(metadataStr), filen.CurrentMasterKey())
	if err != nil {
		return nil, err
	}

	// hash name
	nameHashed := hex.EncodeToString(crypto.RunSHA521([]byte(name)))

	// send
	response, err := filen.client.CreateDirectory(directoryUUID, metadataEncrypted, nameHashed, parentUUID)
	if err != nil {
		return nil, err
	}
	return &Directory{
		UUID:       response.UUID,
		Name:       name,
		ParentUUID: parentUUID,
		Color:      "",
		Created:    time.Now(),
		Favorited:  false,
	}, nil
}

// TrashDirectory moves a directory to trash.
func (filen *Filen) TrashDirectory(uuid string) error {
	return filen.client.TrashDirectory(uuid)
}
