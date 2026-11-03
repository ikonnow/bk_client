/*##### BEGIN GPL LICENSE BLOCK #####

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU General Public License
  as published by the Free Software Foundation; either version 2
  of the License, or (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.

  You should have received a copy of the GNU General Public License
  along with this program; if not, write to the Free Software Foundation,
  Inc., 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA.

##### END GPL LICENSE BLOCK #####*/

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/gookit/color"
)

func assetDownloadHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Ensure the body is closed after reading
	defer r.Body.Close()

	var downloadData DownloadData
	err = json.Unmarshal(body, &downloadData)
	if err != nil {
		fmt.Println(">> Error parsing DownloadRequest:", err)
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Need to populate as DownloadAssetData is sent to bg_unpack.py
	downloadData.DownloadAssetData.Resolution = downloadData.Resolution

	var rJSON map[string]interface{}
	err = json.Unmarshal(body, &rJSON)
	if err != nil {
		fmt.Println(">> Error parsing JSON:", err)
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	taskID := uuid.New().String()

	go doAssetDownload(
		rJSON,
		taskID,
		downloadData.AppID,
		downloadData.Preferences.SceneID,
		downloadData.Preferences.APIKey,
		downloadData.AddonVersion,
		downloadData.PlatformVersion,
		downloadData.DownloadAssetData,
		downloadData.DownloadDirs,
		downloadData.Preferences.UnpackFiles,
		downloadData.Preferences.CreateAssetLibrary,
		downloadData.Preferences.BinaryPath,
		downloadData.Preferences.AddonDir,
		downloadData.Preferences.AddonModuleName,
	)

	// Response to add-on
	resData := map[string]string{"task_id": taskID}
	responseJSON, err := json.Marshal(resData)
	if err != nil {
		http.Error(w, "Error converting to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Ensure the target directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

func syncDirs(sourceDir, targetDir string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories directly, but ensure they exist in the target
		if info.IsDir() {
			var targetPath string
			if len(path) > len(sourceDir) {
				targetPath = filepath.Join(targetDir, path[len(sourceDir):])
			} else {
				targetPath = targetDir
			}
			return os.MkdirAll(targetPath, info.Mode())
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relPath)

		// Check if the file exists in the target directory and if it needs updating
		targetInfo, err := os.Stat(targetPath)
		if os.IsNotExist(err) || info.ModTime().After(targetInfo.ModTime()) {
			if err := copyFile(path, targetPath); err != nil {
				return err
			}
		}
		return nil
	})
}

func syncDirsBidirectional(sourceDir, targetDir string) error {
	err := syncDirs(sourceDir, targetDir)
	if err != nil {
		return err
	}
	return syncDirs(targetDir, sourceDir)
}

// extractZip extracts zipPath into targetDir.
func extractZip(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		path := filepath.Join(targetDir, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func doAssetDownload(
	origJSON map[string]interface{},
	taskID string,
	appID int,
	sceneID string,
	apiKey string,
	addonVersion string,
	platformVersion string,
	downloadAssetData DownloadAssetData,
	downloadDirs []string,
	unpackFiles bool,
	createAssetLibrary bool,
	binaryPath string,
	addonDir string,
	addonModuleName string,
) {
	TasksMux.Lock()
	task := NewTask(origJSON, appID, taskID, "asset_download")
	task.Message = "Getting download URL"
	Tasks[task.AppID][taskID] = task
	TasksMux.Unlock()

	// GET URL FOR BLEND FILE WITH CORRECT RESOLUTION
	selectedFile, _ := GetResolutionFile(downloadAssetData.Files, downloadAssetData.Resolution)
	downloadURLs, fileUploadSize, err := GetSignedURLs(task.Ctx, sceneID, selectedFile, apiKey, addonVersion, platformVersion, appID)
	if err != nil {
		TaskErrorCh <- &TaskError{
			AppID:  appID,
			TaskID: taskID,
			Error:  err}
		return
	}
	downloadURL := downloadURLs[0]

	// EXTRACT FILENAME FROM URL
	TaskProgressUpdateCh <- &TaskProgressUpdate{
		AppID:    appID,
		TaskID:   taskID,
		Progress: 0,
		Message:  "Extracting filename",
	}
	fileName, err := ExtractFilenameFromURL(downloadURL)
	if err != nil {
		TaskErrorCh <- &TaskError{
			AppID:  appID,
			TaskID: taskID,
			Error:  err,
		}
		return
	}

	// GET FILEPATHS TO WHICH WE DOWNLOAD
	TaskProgressUpdateCh <- &TaskProgressUpdate{
		AppID:    appID,
		TaskID:   taskID,
		Progress: 0,
		Message:  "Getting filepaths",
	}
	downloadFilePaths := GetDownloadFilepaths(downloadAssetData, downloadDirs, fileName)
	// Keep a copy for result we send back to add-on (may differ when we extract archives)
	resultFilePaths := make([]string, len(downloadFilePaths))
	copy(resultFilePaths, downloadFilePaths)

	// CHECK IF FILE EXISTS ON HARD DRIVE
	TaskProgressUpdateCh <- &TaskProgressUpdate{
		AppID:    appID,
		TaskID:   taskID,
		Progress: 0,
		Message:  "Checking files on disk",
	}
	existingFiles := []string{}
	for _, filePath := range downloadFilePaths {
		exists, info, err := FileExists(filePath)
		if err != nil {
			if info.IsDir() {
				fmt.Println("Deleting directory:", filePath)
				err := os.RemoveAll(filePath)
				if err != nil {
					fmt.Println("Error deleting directory:", err)
				}
			} else {
				fmt.Println("Error checking if file exists:", err)
			}
			continue
		}
		if exists {
			existingFiles = append(existingFiles, filePath)
		}
	}

	action := ""
	switch len(existingFiles) {
	case 0: // No existing files -> download
		action = "download"
	case 2: // Both files exist -> skip download
		action = "place"
	case 1:
		if len(downloadFilePaths) == 2 { // One file exists, but there are two download paths -> sync the missing file
			action = "sync"
		} else if len(downloadFilePaths) == 1 { // One file exists, and there is only one download path -> skip download
			action = "place"
		}
	default: // Something unexpected happened -> delete and download
		BKLog.Printf("%s Unexpected number of existing files: %s", EmoWarning, existingFiles)
		for _, file := range downloadFilePaths {
			err := DeleteFile(file)
			if err != nil {
				BKLog.Printf("%s Error deleting file: %v", EmoWarning, err)
			}
		}
	}

	// START DOWNLOAD IF NEEDED
	fp := downloadFilePaths[0]
	if action == "download" {
		downloadURL, err = downloadAssetSources(downloadURLs, fp, fileUploadSize, appID, addonVersion, platformVersion, taskID, task.Ctx)
		if err != nil {
			e := fmt.Errorf("error downloading asset: %w", err)
			TaskErrorCh <- &TaskError{
				AppID:  appID,
				TaskID: taskID,
				Error:  e,
			}
			return
		}
	} else {
		fmt.Println("PLACING THE FILE")
	}

	// If the downloaded (or placed) file is a packaged archive, extract it always and use the contained .blend
	// add-ons are never extracted, since blender needs them as .zip
	isZip := strings.HasSuffix(strings.ToLower(fileName), ".zip")
	if isZip && downloadAssetData.AssetType != "addon" {
		// Ensure fp points to the existing zip in both branches
		if action != "download" {
			fp = existingFiles[0]
		}
		targetDir := filepath.Dir(fp)
		TaskMessageCh <- &TaskMessageUpdate{AppID: appID, TaskID: taskID, Message: "Extracting archive"}
		if err := extractZip(fp, targetDir); err != nil {
			TaskErrorCh <- &TaskError{AppID: appID, TaskID: taskID, Error: fmt.Errorf("failed to extract archive: %w", err)}
			return
		}
		// Best-effort: remove the archive after extraction
		_ = os.Remove(fp)
		// Find a .blend in targetDir (root)
		var blendPath string
		if matches, _ := filepath.Glob(filepath.Join(targetDir, "*.blend")); len(matches) > 0 {
			blendPath = matches[0]
		}
		if blendPath == "" {
			TaskErrorCh <- &TaskError{AppID: appID, TaskID: taskID, Error: fmt.Errorf("archive does not contain a .blend at root")}
			return
		}
		// Switch to extracted .blend for next steps and result
		fp = blendPath
		resultFilePaths = []string{fp}
	}

	// UNPACKING / METADATA WRITE
	// Trigger unpack only on fresh downloads. The unpack script opens the .blend in a
	// background Blender and re-saves it (bpy.ops.wm.save_as_mainfile), which upgrades
	// the file format to match that Blender version. Re-running unpack on an already
	// existing file ("place" or "sync") with a different Blender version would make the
	// .blend unreadable by older Blenders.
	shouldUnpack := (unpackFiles || createAssetLibrary) && action == "download"
	if shouldUnpack {
		//err := UnpackAsset(fp, data, taskID)
		assetDataRaw, _ := origJSON["asset_data"]

		err := UnpackAsset(
			fp,
			taskID,
			appID,
			downloadAssetData.AssetType,
			binaryPath,
			addonDir,
			addonModuleName,
			assetDataRaw,
			unpackFiles,
			createAssetLibrary,
		)
		if err != nil {
			BKLog.Printf("%s error unpacking asset (non-fatal, asset will still be placed): %v", EmoWarning, err)
			TaskMessageCh <- &TaskMessageUpdate{
				AppID:   appID,
				TaskID:  taskID,
				Message: fmt.Sprintf("Unpacking failed (asset will still be placed): %v", err),
			}
		}
	}

	// SYNC FILES IF NEEDED
	if action == "sync" || (action == "download" && len(downloadFilePaths) == 2) {
		//Synchronize both folders - we need to synchronize both sides.
		// In both folders there can be different resolutions, packed or unpacked with subfolders.

		// get directory filepaths from the downloadFilePaths
		globalAssetDir := filepath.Dir(downloadFilePaths[0])
		localAssetDir := filepath.Dir(downloadFilePaths[1])

		// Synchronize bidirectional
		err := syncDirsBidirectional(localAssetDir, globalAssetDir)
		if err != nil {
			e := fmt.Errorf("error synchronizing global and local folders: %w", err)
			TaskErrorCh <- &TaskError{
				AppID:  appID,
				TaskID: taskID,
				Error:  e,
			}
			return
		}

	}

	result := map[string]interface{}{"file_paths": resultFilePaths, "url": downloadURL}
	TaskFinishCh <- &TaskFinish{
		AppID:   appID,
		TaskID:  taskID,
		Message: "Asset downloaded and ready",
		Result:  result,
	}
}

// UnpackAsset unpacks the downloaded asset (.blend file).
// It skips unpacking for HDRi files and returns nil immediately.
func UnpackAsset(
	blendPath string,
	taskID string,
	appID int,
	assetType string,
	binaryPath string,
	addonDir string,
	addonModuleName string,
	assetDataRaw interface{},
	unpackFiles bool,
	createAssetLibrary bool,
	//prefs PREFS,
) error {
	// Unpacking and metadata write require opening a .blend in background Blender.
	// Skip safely for raw files (e.g. .hdr) to avoid startup/unpack errors.
	if !strings.EqualFold(filepath.Ext(blendPath), ".blend") {
		TaskMessageCh <- &TaskMessageUpdate{
			AppID:   appID,
			TaskID:  taskID,
			Message: fmt.Sprintf("Skipping unpack: downloaded file is not a .blend (%s)", filepath.Ext(blendPath)),
		}
		return nil
	}

	// Microsoft Store Blender installs live under WindowsApps which blocks
	// external processes from exec-ing the binary ("access is denied").
	// Detect this early and skip unpack with a clear message.
	if runtime.GOOS == "windows" && strings.Contains(strings.ToLower(binaryPath), "windowsapps") {
		TaskMessageCh <- &TaskMessageUpdate{
			AppID:   appID,
			TaskID:  taskID,
			Message: "Skipping unpack: Microsoft Store Blender cannot be launched as background process",
		}
		return nil
	}

	TaskMessageCh <- &TaskMessageUpdate{
		AppID:   appID,
		TaskID:  taskID,
		Message: "Unpacking files",
	}
	blenderUserScripts := filepath.Dir(filepath.Dir(addonDir)) // e.g.: /Users/username/Library/Application Support/Blender/4.1/scripts"
	unpackScriptPath := filepath.Join(addonDir, "unpack_asset_bg.py")
	dataFile := filepath.Join(os.TempDir(), "resdata.json")

	process_data := map[string]interface{}{
		"fpath":      blendPath,
		"asset_data": assetDataRaw,
		"command":    "unpack",
		"prefs": map[string]interface{}{
			"unpack_files":         unpackFiles,
			"create_asset_library": createAssetLibrary,
		},
	}
	jsonData, err := json.Marshal(process_data)
	if err != nil {
		return err
	}
	err = os.WriteFile(dataFile, jsonData, 0644)
	if err != nil {
		return err
	}

	cmd := exec.Command(
		binaryPath,
		"--background",
		"--factory-startup", // disables user preferences, addons, etc.
		"-noaudio",
		blendPath,
		"--python", unpackScriptPath,
		"--",
		dataFile,
		addonModuleName, // Legacy has it as "blenderkit", extensions have it like bl_ext.user_default.blenderkit or anything else
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("BLENDER_USER_SCRIPTS=%v", blenderUserScripts))

	// Redirect both stdout and stderr to the buffer
	var combinedOutput bytes.Buffer
	cmd.Stdout = &combinedOutput
	cmd.Stderr = &combinedOutput

	err = cmd.Run()
	color.FgGray.Printf("└> background unpacking '%+v' logs:\n", cmd)
	for _, line := range strings.Split(combinedOutput.String(), "\n") {
		color.FgGray.Printf("   %s\n", line)
	}
	if err != nil {
		// Include Blender's output in the error so callers can see why it failed
		// (e.g. "not a blend file" when old Blender can't read a newer .blend format).
		output := strings.TrimSpace(combinedOutput.String())
		if output != "" {
			return fmt.Errorf("%w, Blender output: %s", err, output)
		}
		return err
	}

	return nil
}

func downloadAsset(url, filePath string, uploadFileSize int, appID int, addonVersion, platformVersion, taskID string, ctx context.Context) error {
	_, err := downloadAssetSources([]string{url}, filePath, uploadFileSize, appID, addonVersion, platformVersion, taskID, ctx)
	return err
}

func downloadAssetSources(urls []string, filePath string, uploadFileSize int, appID int, addonVersion, platformVersion, taskID string, ctx context.Context) (string, error) {
	headers := getHeaders("", *SystemID, addonVersion, platformVersion, appID)
	headers.Set("Cookie", "allow_compression=true")
	progress := func(downloaded, total int64) {
		percentage := 0
		message := "Downloading"
		if total > 0 {
			percentage = int(100 * downloaded / total)
			if percentage > 100 {
				percentage = 100
			}
			message = fmt.Sprintf("Downloading %.1fMB (%d%%)", float64(total)/1024/1024, percentage)
		} else if downloaded > 0 {
			message = fmt.Sprintf("Downloaded %.1fMB", float64(downloaded)/1024/1024)
		}
		select {
		case TaskProgressUpdateCh <- &TaskProgressUpdate{AppID: appID, TaskID: taskID, Progress: percentage, Message: message}:
		case <-ctx.Done():
		}
	}
	return downloadFromSources(ctx, ClientDownloads, urls, filePath, int64(uploadFileSize), headers, progress)
}

// should return ['/Users/ag/blenderkit_data/models/kitten_0992088b-fb84-4c69-bb6e-426272970c8b/kitten_2K_d5368c9d-092e-4319-afe1-dd765de6da01.blend']
func GetDownloadFilepaths(downloadAssetData DownloadAssetData, downloadDirs []string, filename string) []string {
	filePaths := []string{}
	filename = ServerToLocalFilename(filename, downloadAssetData.Name)
	assetDirName := GetAssetDirectoryName(downloadAssetData.Name, downloadAssetData.ID)
	for _, dir := range downloadDirs {
		assetDirPath := filepath.Join(dir, assetDirName)
		if _, err := os.Stat(assetDirPath); os.IsNotExist(err) {
			os.MkdirAll(assetDirPath, os.ModePerm)
		}
		filePath := filepath.Join(assetDirPath, filename)
		filePaths = append(filePaths, filePath)
	}

	// For Mac and Linux, we can return the file paths as they are.
	if runtime.GOOS != "windows" {
		return filePaths
	}

	// On Windows we need to check if path is not too long
	var filteredWinPaths []string
	for _, filePath := range filePaths {
		if len(filePath) < 259 {
			filteredWinPaths = append(filteredWinPaths, filePath)
		}
	}
	return filteredWinPaths
}

// GetSignedURL fetches a signed CDN URL for a pre-selected AssetFile.
// canDownload - true if the file can be downloaded, false otherwise
// downloadURL - https://assets.blendkit.com/assets/.../file.blend?verify=...
// fileUploadSize - size of the file to download
func GetSignedURL(sceneID string, file AssetFile, apiKey, addonVersion, platformVersion string, appID int) (bool, string, int, error) {
	urls, size, err := GetSignedURLs(context.Background(), sceneID, file, apiKey, addonVersion, platformVersion, appID)
	if err != nil {
		return false, "", 0, err
	}
	source, err := selectReachableDownloadURL(context.Background(), ClientDownloads, urls, getHeaders("", *SystemID, addonVersion, platformVersion, appID))
	return err == nil, source, size, err
}

func GetSignedURLs(ctx context.Context, sceneID string, file AssetFile, apiKey, addonVersion, platformVersion string, appID int) ([]string, int, error) {
	reqData := url.Values{}
	reqData.Set("scene_uuid", sceneID)

	req, err := http.NewRequestWithContext(ctx, "GET", file.DownloadURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid download metadata URL")
	}
	req.Header = getHeaders(apiKey, *SystemID, addonVersion, platformVersion, appID)
	req.URL.RawQuery = reqData.Encode()

	resp, err := ClientAPI.Do(req)
	if err != nil {
		return nil, 0, safeDownloadError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("download metadata returned HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	urls, err := parseDownloadURLs(bodyBytes)
	return urls, file.FileUploadSize, err
}

// GetDownloadURL selects the best file for the given resolution and fetches a signed CDN URL.
// Kept for Blender add-on compatibility.
func GetDownloadURL(sceneID string, files []AssetFile, resolution string, apiKey, addonVersion, platformVersion string, appID int) (bool, string, int, error) {
	file, _ := GetResolutionFile(files, resolution)
	return GetSignedURL(sceneID, file, apiKey, addonVersion, platformVersion, appID)
}

// selectAssetFile picks the best AssetFile using the modelFormat preference,
// falling back to .blend at the given resolution. GLTF is only attempted for model assets.
func selectAssetFile(files []AssetFile, assetType, modelFormat, resolution string) (AssetFile, string) {
	if assetType == "model" {
		var candidates []string
		switch modelFormat {
		case "gltf_godot":
			candidates = []string{"gltf_godot", "gltf"}
		case "gltf":
			candidates = []string{"gltf", "gltf_godot"}
		}
		for _, ft := range candidates {
			if f, got := GetResolutionFile(files, ft); got == ft {
				return f, got
			}
		}
		if candidates != nil {
			BKLog.Printf("%s No GLTF file available for asset, falling back to .blend at %s", EmoWarning, resolution)
		}
	}
	return GetResolutionFile(files, resolution)
}

func GetResolutionFile(files []AssetFile, targetRes string) (AssetFile, string) {
	resolutionsMap := map[string]int{
		"resolution_0_5K": 512,
		"resolution_1K":   1024,
		"resolution_2K":   2048,
		"resolution_4K":   4096,
		"resolution_8K":   8192,
	}
	var originalFile, closest, zipFile AssetFile
	var targetResInt, mindist = resolutionsMap[targetRes], 100000000

	fmt.Println(">> Target resolution:", targetRes)
	for _, f := range files {
		fmt.Println(">> File type:", f.FileType)
		if f.FileType == "thumbnail" {
			continue
		}
		if f.FileType == "blend" {
			originalFile = f
			if targetRes == "ORIGINAL" {
				return f, "blend"
			}
		}
		if f.FileType == "zip_file" {
			zipFile = f
		}
		if f.FileType == targetRes {
			return f, f.FileType
		}

		// No exact match - select resolution closest to the target.
		rval, ok := resolutionsMap[f.FileType]
		if ok && targetResInt != 0 {
			rdiff := abs(targetResInt - rval)
			if rdiff < mindist {
				closest = f
				mindist = rdiff
			}
		}
	}

	if (closest != AssetFile{}) {
		return closest, closest.FileType
	}
	if (zipFile != AssetFile{}) {
		return zipFile, "zip_file"
	}

	return originalFile, "blend"
}
