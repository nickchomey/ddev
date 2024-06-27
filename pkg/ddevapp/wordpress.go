package ddevapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ddev/ddev/pkg/archive"
	"github.com/ddev/ddev/pkg/fileutil"
	"github.com/ddev/ddev/pkg/nodeps"
	"github.com/ddev/ddev/pkg/util"
	copy2 "github.com/otiai10/copy"
)

// WordpressConfig encapsulates all the configurations for a WordPress site.
type WordpressConfig struct {
	WPGeneric        bool
	DeployName       string
	DeployURL        string
	DatabaseName     string
	DatabaseUsername string
	DatabasePassword string
	DatabaseHost     string
	AuthKey          string
	SecureAuthKey    string
	LoggedInKey      string
	NonceKey         string
	AuthSalt         string
	SecureAuthSalt   string
	LoggedInSalt     string
	NonceSalt        string
	Docroot          string
	TablePrefix      string
	Signature        string
	SiteSettings     string
	SiteSettingsDdev string
	AbsPath          string
	DbCharset        string
	DbCollate        string
}

// NewWordpressConfig produces a WordpressConfig object with defaults.
func NewWordpressConfig(app *DdevApp, absPath string) *WordpressConfig {
	return &WordpressConfig{
		WPGeneric:        false,
		DatabaseName:     "db",
		DatabaseUsername: "db",
		DatabasePassword: "db",
		DatabaseHost:     "db",
		DeployURL:        app.GetPrimaryURL(),
		Docroot:          "/var/www/html/docroot",
		TablePrefix:      "wp_",
		AuthKey:          util.RandString(64),
		AuthSalt:         util.RandString(64),
		LoggedInKey:      util.RandString(64),
		LoggedInSalt:     util.RandString(64),
		NonceKey:         util.RandString(64),
		NonceSalt:        util.RandString(64),
		SecureAuthKey:    util.RandString(64),
		SecureAuthSalt:   util.RandString(64),
		Signature:        nodeps.DdevFileSignature,
		SiteSettings:     "wp-config.php",
		SiteSettingsDdev: "wp-config-development.php",
		AbsPath:          absPath,
		DbCharset:        "utf8mb4",
		DbCollate:        "",
	}
}

// wordPressHooks adds a wp-specific hooks example for post-start
const wordPressHooks = `# Un-comment to emit the WP CLI version after ddev start.
#  post-start:
#    - exec: wp cli version
`

// getWordpressHooks for appending as byte array
func getWordpressHooks() []byte {
	return []byte(wordPressHooks)
}

// getWordpressUploadDirs will return the default paths.
func getWordpressUploadDirs(_ *DdevApp) []string {
	return []string{"wp-content/uploads"}
}

const wordpressConfigInstructions = `
An existing user-managed wp-config.php file has been detected!
Project DDEV settings have been written to:

%s

Please comment out any database connection settings in your wp-config.php and
add the following snippet to your wp-config.php, near the bottom of the file
and before the include of wp-settings.php:

// Include for ddev-managed settings in wp-config-development.php.
$ddev_settings = dirname(__FILE__) . '/wp-config-development.php';
if (is_readable($ddev_settings) && !defined('DB_USER')) {
  require_once($ddev_settings);
}

If you don't care about those settings, or config is managed in a .env
file, etc, then you can eliminate this message by putting a line that says
// wp-config-development.php not needed
in your wp-config.php
`

// createWordpressSettingsFile creates a Wordpress settings file from a
// template. Returns full path to location of file + err
func createWordpressSettingsFile(app *DdevApp) (string, error) {
	absPath, err := wordpressGetRelativeAbsPath(app)
	if err != nil {
		if strings.Contains(err.Error(), "multiple") {
			util.Warning("Unable to determine ABSPATH: %v", err)
		}
	}

	config := NewWordpressConfig(app, absPath)

	// Write DDEV settings file
	if err := writeWordpressDdevSettingsFile(config, app.SiteDdevSettingsFile); err != nil {
		return "", err
	}

	if ! fileutil.FileExists(app.SiteSettingsPath) {
		
		// If settings file does not exist, write basic settings file including it
		if err := writeWordpressSettingsFile(config, app.SiteSettingsPath); err != nil {
			return "", err
		}
	}

	return app.SiteDdevSettingsFile, nil
}

// writeWordpressSettingsFile dynamically produces valid wp-config.php file by combining a configuration
// object with a data-driven template.
func writeWordpressSettingsFile(wordpressConfig *WordpressConfig, filePath string) error {
	t, err := template.New("wp-config.php").ParseFS(bundledAssets, "wordpress/wp-config.php")
	if err != nil {
		return err
	}

	// Ensure target directory exists and is writable
	dir := filepath.Dir(filePath)
	if err = util.Chmod(dir, 0755); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer util.CheckClose(file)

	//nolint: revive
	if err = t.Execute(file, wordpressConfig); err != nil {
		return err
	}

	return nil
}

// writeWordpressDdevSettingsFile unconditionally creates the file that contains ddev-specific settings.
func writeWordpressDdevSettingsFile(config *WordpressConfig, filePath string) error {
	if fileutil.FileExists(filePath) {
		// Check if the file is managed by ddev.
		signatureFound, err := fileutil.FgrepStringInFile(filePath, nodeps.DdevFileSignature)
		if err != nil {
			return err
		}

		// If the signature wasn't found, warn the user and return.
		if !signatureFound {
			util.Warning("%s already exists and is managed by the user.", filepath.Base(filePath))
			return nil
		}
	}

	t, err := template.New("wp-config-development.php").ParseFS(bundledAssets, "wordpress/wp-config-development.php")
	if err != nil {
		return err
	}

	// Ensure target directory exists and is writable
	dir := filepath.Dir(filePath)
	if err = util.Chmod(dir, 0755); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer util.CheckClose(file)

	err = t.Execute(file, config)
	return err
}

// setWordpressSiteSettingsPaths sets the expected settings files paths for
// a WordPress site.
func setWordpressSiteSettingsPaths(app *DdevApp) {
	config := NewWordpressConfig(app, "")

	settingsFileBasePath := filepath.Join(app.AppRoot, app.Docroot)
	app.SiteSettingsPath = filepath.Join(settingsFileBasePath, config.SiteSettings)
	app.SiteDdevSettingsFile = filepath.Join(settingsFileBasePath, config.SiteSettingsDdev)
}

// isWordpressApp returns true if the app of of type wordpress
func isWordpressApp(app *DdevApp) bool {
	_, err := wordpressGetRelativeAbsPath(app)
	if err != nil {
		// Multiple abspath candidates is an issue, but is still a valid
		// indicator that this is a WordPress app
		return strings.Contains(err.Error(), "multiple")

	}

	return true
}

// wordpressImportFilesAction defines the Wordpress workflow for importing project files.
// The Wordpress workflow is currently identical to the Drupal import-files workflow.
func wordpressImportFilesAction(app *DdevApp, target, importPath, extPath string) error {
	destPath := app.calculateHostUploadDirFullPath(target)

	// Parent of destination dir should exist
	if !fileutil.FileExists(filepath.Dir(destPath)) {
		return fmt.Errorf("unable to import to %s: parent directory does not exist", destPath)
	}

	// Parent of destination dir should be writable.
	if err := util.Chmod(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	// If the destination path exists, purge it as was warned
	if fileutil.FileExists(destPath) {
		if err := fileutil.PurgeDirectory(destPath); err != nil {
			return fmt.Errorf("failed to cleanup %s before import: %v", destPath, err)
		}
	}

	if isTar(importPath) {
		if err := archive.Untar(importPath, destPath, extPath); err != nil {
			return fmt.Errorf("failed to extract provided archive: %v", err)
		}

		return nil
	}

	if isZip(importPath) {
		if err := archive.Unzip(importPath, destPath, extPath); err != nil {
			return fmt.Errorf("failed to extract provided archive: %v", err)
		}

		return nil
	}

	if err := copy2.Copy(importPath, destPath); err != nil {
		return err
	}

	return nil
}

// wordpressGetRelativeAbsPath returns the portion of the ABSPATH value that will come after "/" in wp-config.php -
// this is done by searching (at a max depth of one directory from the docroot) for wp-settings.php, the
// file we're using as a signal to indicate that this is a WordPress project.
func wordpressGetRelativeAbsPath(app *DdevApp) (string, error) {
	needle := "wp-settings.php"

	curDirMatches, err := filepath.Glob(filepath.Join(app.AppRoot, app.Docroot, needle))
	if err != nil {
		return "", err
	}

	if len(curDirMatches) > 0 {
		return "", nil
	}

	subDirMatches, err := filepath.Glob(filepath.Join(app.AppRoot, app.Docroot, "*", needle))
	if err != nil {
		return "", err
	}

	if len(subDirMatches) == 0 {
		return "", fmt.Errorf("unable to find %s in subdirectories", needle)
	}

	if len(subDirMatches) > 1 {
		return "", fmt.Errorf("multiple subdirectories contain %s", needle)
	}

	absPath := filepath.Base(filepath.Dir(subDirMatches[0]))

	return absPath, nil
}

// wordpressConfigOverrideAction sets custom parameters that are needed for a WP app to work properly
func wordpressConfigOverrideAction(app *DdevApp) error {
	// This allows a standard wp-config.php to choose between loading from wp-config-development or using default parameters. It is also the standard mechanism that plugins use to toggle behaviour based on what sort of environment is currrently running
	app.WebEnvironment = []string{"WP_ENVIRONMENT_TYPE=development", "WPURL=site.com"}
	
	// set php to 8.1 because it has better compatibility with Wordpress 
    // as of WP 6.5, php 8.1 is "compatible, with exceptions" while 8.2 has "beta support"
    // https://make.wordpress.org/core/handbook/references/php-compatibility-and-wordpress-versions/
	app.PHPVersion = "8.1"
	
	
	
	/* if app.WebserverType == nodeps.WebserverDefault {
		app.WebserverType = nodeps.WebserverNginxGunicorn
	}
	if app.Database == DatabaseDefault {
		app.Database.Type = nodeps.Postgres
		app.Database.Version = nodeps.Postgres14
	} */
	return nil
}

// wordpressPostConfigAction 
func wordpressPostConfigAction(app *DdevApp) error {
	
	// create a working copy of the default middleware-template.yaml.example file
	
	// Read the entire source file into memory
    data, err := os.ReadFile(app.GetConfigPath(".middleware-template.yaml.example"))
    if err != nil {
        return err
    }

    // Write the data to the destination file
    // The permissions are set to 0644, but you can adjust as needed
    err = os.WriteFile(app.GetConfigPath(".middleware-template.yaml"), data, 0644)
    if err != nil {
        return err
    }

    return nil
}

// wordpressPostStartAction 
func wordpressPostStartAction(app *DdevApp) error {
	
	/* need to see if this runs before or after traefik router is created
	
	
	
	
	
	
	
	/* if app.DisableSettingsManagement {
		return nil
	}
	envFilePath := filepath.Join(app.AppRoot, ".env.local")
	_, envText, err := ReadProjectEnvFile(envFilePath)
	var envMap = map[string]string{
		"DATABASE_URL": `mysql://db:db@db:3306/db`,
		"APP_ENV":      "dev",
		"APP_URL":      app.GetPrimaryURL(),
		"MAILER_DSN":   `smtp://127.0.0.1:1025?encryption=&auth_mode=`,
	}
	// If the .env.local doesn't exist, create it.
	switch {
	case err == nil:
		util.Warning("Updating %s with %v", envFilePath, envMap)
		fallthrough
	case errors.Is(err, os.ErrNotExist):
		err := WriteProjectEnvFile(envFilePath, envMap, envText)
		if err != nil {
			return err
		}
	default:
		util.Warning("error opening %s: %v", envFilePath, err)
	}
	*/
	return nil 
}