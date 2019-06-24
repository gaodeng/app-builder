package electron

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"

	"github.com/alecthomas/kingpin"
	"github.com/apex/log"
	"github.com/develar/app-builder/pkg/archive/zipx"
	"github.com/develar/app-builder/pkg/util"
	"github.com/develar/go-fs-util"
)

func ConfigureUnpackCommand(app *kingpin.Application) {
	command := app.Command("unpack-electron", "")
	jsonConfig := command.Flag("configuration", "").Short('c').Required().String()
	outputDir := command.Flag("output", "").Required().String()
	distMacOsAppName := command.Flag("distMacOsAppName", "").Default("Electron.app").String()

	command.Action(func(context *kingpin.ParseContext) error {
		var configs []ElectronDownloadOptions
		err := util.DecodeBase64IfNeeded(*jsonConfig, &configs)
		if err != nil {
			return err
		}
		return UnpackElectron(configs, *outputDir, *distMacOsAppName, true)
	})
}

func UnpackElectron(configs []ElectronDownloadOptions, outputDir string, distMacOsAppName string, isReDownloadOnFileReadError bool) error {
	cachedElectronZip := make(chan string, 1)
	err := util.MapAsync(2, func(taskIndex int) (func() error, error) {
		if taskIndex == 0 {
			return func() error {
				return fsutil.EnsureEmptyDir(outputDir)
			}, nil
		} else {
			return func() error {
				result, err := downloadElectron(configs)
				if err != nil {
					return err
				}

				cachedElectronZip <- result[0]
				return nil
			}, nil
		}
	})

	if err != nil {
		return err
	}

	if len(distMacOsAppName) == 0 {
		distMacOsAppName = "Electron"
	}

	excludedFiles := make(map[string]bool)
	excludedFiles[filepath.Join(outputDir, distMacOsAppName, "Contents", "Resources", "default_app.asar")] = true
	excludedFiles[filepath.Join(outputDir, "resources", "default_app.asar")] = true

	excludedFiles[filepath.Join(outputDir, distMacOsAppName, "Contents", "Resources", "inspector", ".htaccess")] = true
	excludedFiles[filepath.Join(outputDir, "resources", "inspector", ".htaccess")] = true

	excludedFiles[filepath.Join(outputDir, "version")] = true

	zipFile := <-cachedElectronZip
	err = zipx.Unzip(zipFile, outputDir, excludedFiles)
	if err != nil {
		if isReDownloadOnFileReadError && (err == zip.ErrFormat || err == io.ErrUnexpectedEOF) {
			log.WithError(err).Warn("cannot unpack electron zip file, will be re-downloaded")
			// not just download and unzip, but full - including clearing of output dir
			err = os.Remove(zipFile)
			if err != nil && !os.IsNotExist(err) {
				log.WithError(err).WithField("file", zipFile).Warn("cannot delete")
			}

			return UnpackElectron(configs, outputDir, distMacOsAppName, false)
		} else {
			return err
		}
	}

	return nil
}
