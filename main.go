// TTW Software Team
// Mathis Van Eetvelde
// 2021-present

// Modified by Aditya Karnam
// 2021
// Added file overwrite support

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sethvargo/go-githubactions"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

const (
	scope                 = "https://www.googleapis.com/auth/drive.file"
	filenameInput         = "filename"
	nameInput             = "name"
	folderIdInput         = "folderId"
	credentialsInput      = "credentials"
	overwrite             = "false"
	mimeTypeInput         = "mimeType"
	useCompleteSourceName = "useCompleteSourceFilenameAsName"
	namePrefixInput       = "namePrefix"
)

func uploadToDrive(svc *drive.Service, filename string, folderId string, driveFile *drive.File, name string, mimeType string) {
	file, err := os.Open(filename)
	if err != nil {
		githubactions.Fatalf(fmt.Sprintf("opening file with filename: %v failed with error: %v", filename, err))
	}

	if driveFile != nil {
		f := &drive.File{
			Name:     name,
			MimeType: mimeType,
		}
		_, err = svc.Files.Update(driveFile.Id, f).AddParents(folderId).Media(file).SupportsAllDrives(true).Do()
	} else {
		f := &drive.File{
			Name:     name,
			MimeType: mimeType,
			Parents:  []string{folderId},
		}
		_, err = svc.Files.Create(f).Media(file).SupportsAllDrives(true).Do()
	}

	if err != nil {
		githubactions.Fatalf(fmt.Sprintf("creating/updating file failed with error: %v", err))
	} else {
		githubactions.Debugf("Uploaded/Updated file.")
	}
}

func main() {

	// get filename argument from action input
	filename := githubactions.GetInput(filenameInput)
	if filename == "" {
		missingInput(filenameInput)
	}
	files, err := filepath.Glob(filename)
	fmt.Printf("Files: %v\n", files)
	if err != nil {
		githubactions.Fatalf(fmt.Sprintf("Invalid filename pattern: %v", err))
	}
	if len(files) == 0 {
		githubactions.Fatalf(fmt.Sprintf("No file found! pattern: %s", filename))
	}

	// get overwrite flag
	var overwriteFlag bool
	overwrite := githubactions.GetInput("overwrite")
	if overwrite == "" {
		githubactions.Warningf("Overwrite is disabled.")
		overwriteFlag = false
	} else {
		overwriteFlag, _ = strconv.ParseBool(overwrite)
	}
	// get name argument from action input
	name := githubactions.GetInput(nameInput)

	// get folderId argument from action input
	folderId := githubactions.GetInput(folderIdInput)
	if folderId == "" {
		missingInput(folderIdInput)
	}

	// get file mimeType argument from action input
	mimeType := githubactions.GetInput(mimeTypeInput)

	var useCompleteSourceFilenameAsNameFlag bool
	useCompleteSourceFilenameAsName := githubactions.GetInput(useCompleteSourceName)
	if useCompleteSourceFilenameAsName == "" {
		fmt.Println("useCompleteSourceFilenameAsName is disabled.")
		useCompleteSourceFilenameAsNameFlag = false
	} else {
		useCompleteSourceFilenameAsNameFlag, _ = strconv.ParseBool(useCompleteSourceFilenameAsName)
	}

	// get filename prefix
	filenamePrefix := githubactions.GetInput(namePrefixInput)

	// get base64 encoded credentials argument from action input
	credentials := githubactions.GetInput(credentialsInput)
	if credentials == "" {
		missingInput(credentialsInput)
	}
	// add base64 encoded credentials argument to mask
	githubactions.AddMask(credentials)

	// decode credentials to []byte
	decodedCredentials, err := base64.StdEncoding.DecodeString(credentials)
	if err != nil {
		githubactions.Fatalf(fmt.Sprintf("base64 decoding of 'credentials' failed with error: %v", err))
	}

	creds := strings.TrimSuffix(string(decodedCredentials), "\n")

	// add decoded credentials argument to mask
	githubactions.AddMask(creds)

	// fetching a JWT config with credentials and the right scope
	conf, err := google.JWTConfigFromJSON([]byte(creds), scope)
	if err != nil {
		githubactions.Fatalf(fmt.Sprintf("fetching JWT credentials failed with error: %v", err))
	}

	// instantiating a new drive service
	ctx := context.Background()
	svc, err := drive.New(conf.Client(ctx))
	if err != nil {
		log.Println(err)
	}

	useSourceFilename := len(files) > 1

	for _, file := range files {
		var targetName string
		if useCompleteSourceFilenameAsNameFlag {
			targetName = file
		} else if useSourceFilename || name == "" {
			targetName = filepath.Base(file)
		} else {
			targetName = name
		}
		if targetName == "" {
			githubactions.Fatalf("Could not discover target file name")
		} else if filenamePrefix != "" {
			targetName = filenamePrefix + targetName
		}
		uploadFile(svc, file, folderId, targetName, mimeType, overwriteFlag)
	}
}

func uploadFile(svc *drive.Service, filename string, folderId string, name string, mimeType string, overwriteFlag bool) {

	fmt.Printf("target file name: %s\n", name)

	if overwriteFlag {
		r, err := svc.Files.List().Fields("files(name,id,mimeType,parents)").Q("name='" + name + "'").IncludeItemsFromAllDrives(true).Corpora("allDrives").SupportsAllDrives(true).Do()
		if err != nil {
			log.Fatalf("Unable to retrieve files: %v", err)
			fmt.Println("Unable to retrieve files")
		}
		fmt.Printf("Files: %d\n", len(r.Files))
		var currentFile *drive.File = nil
		for _, i := range r.Files {
			found := false
			if name == i.Name {
				currentFile = i
				for _, p := range i.Parents {
					if p == folderId {
						fmt.Println("file found in expected folder")
						found = true
						break
					}
				}
			}
			if found {
				break
			}
		}

		if currentFile == nil {
			fmt.Println("No similar files found. Creating a new file")
			uploadToDrive(svc, filename, folderId, nil, name, mimeType)
		} else {
			fmt.Printf("Overwriting file: %s (%s)\n", currentFile.Name, currentFile.Id)
			uploadToDrive(svc, filename, folderId, currentFile, name, mimeType)
		}
	} else {
		uploadToDrive(svc, filename, folderId, nil, name, mimeType)
	}
}

func missingInput(inputName string) {
	githubactions.Fatalf(fmt.Sprintf("missing input '%v'", inputName))
}
