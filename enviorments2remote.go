package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
  "os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// copyDirectory recursively copies a directory and its contents.
func copyDirectory(src, dest string, wg *sync.WaitGroup) {
	defer wg.Done()

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file contents.
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})

	if err != nil {
		fmt.Printf("Error copying directory from '%s' to '%s': %v\n", src, dest, err)
	}
}

// getDesktopSolutionsPath returns the path to the "Solutions" directory on the desktop.
func getDesktopSolutionsPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "Desktop", "Solutions")
}

// Prompt user if they want to create a GitHub repository
func promptForGitRepo(envDirName string) bool {
	var answer string
	fmt.Printf("Do you want to create a new GitHub repository for the '%s' environment? (y/n): ", envDirName)
	fmt.Scanln(&answer)

	return strings.ToLower(answer) == "y"
}

// Create a GitHub repository using GitHub API
func createGitHubRepo(envDirName string) error {
	token := os.Getenv("GITHUB_TOKEN") // Ensure you have your GitHub token set as an environment variable

	if token == "" {
		return fmt.Errorf("GitHub token is required, but it's not set")
	}

	url := "https://api.github.com/user/repos"
	jsonBody := fmt.Sprintf(`{"name":"%s", "private":true}`, envDirName)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(jsonBody)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create repository, status code: %d", resp.StatusCode)
	}

	fmt.Printf("Successfully created GitHub repository: https://github.com/AlbersSoftware/%s\n", envDirName)
	return nil
}


// Setup git for the environment directory
func setupGitForEnv(envDirName, solutionPath string) error {
	// Initialize Git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = envDirName
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize git repository: %v", err)
	}

	// Add all files to staging area
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = envDirName
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add files: %v", err)
	}

	// Commit the changes
	cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("first commit for %s", envDirName))
	cmd.Dir = envDirName
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	// Set the remote origin
	remoteURL := fmt.Sprintf("https://github.com/AlbersSoftware/%s.git", envDirName)
	cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
	cmd.Dir = filepath.Join(solutionPath, envDirName) // Ensure this is the correct working directory
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set remote origin: %v", err)
	}

	// Push to GitHub
	cmd = exec.Command("git", "push", "-u", "origin", "master")
	cmd.Dir = filepath.Join(solutionPath, envDirName) // Same as above
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push to remote repository: %v", err)
	}

	fmt.Printf("Successfully pushed to %s\n", remoteURL)
	return nil
}


func main() {
	var directoryName, solutionName string

	// Get input for the directory you wish to copy and the outer shell directory name to be placed in the Solutions directory.
	fmt.Print("Enter the directory name you wish to copy. If it's not in the current directory you run this script from, it will need the absolute path: ")
	fmt.Scanln(&directoryName)
	fmt.Print("Enter the solution name for your outer shell directory: ")
	fmt.Scanln(&solutionName)

	// Check if the input directory exists.
	if _, err := os.Stat(directoryName); os.IsNotExist(err) {
		fmt.Println("The specified directory does not exist. Did you use the absolute path?")
		return
	}

	// Create the "Solutions" directory if it doesn't exist.
	solutionsPath := getDesktopSolutionsPath()
	if _, err := os.Stat(solutionsPath); os.IsNotExist(err) {
		err := os.MkdirAll(solutionsPath, 0755)
		if err != nil {
			fmt.Printf("Failed to create 'Solutions' directory: %v\n", err)
			return
		}
	}

	// Create the solution directory.
	solutionPath := filepath.Join(solutionsPath, solutionName)
	err := os.MkdirAll(solutionPath, 0755)
	if err != nil {
		fmt.Printf("Failed to create solution directory: %v\n", err)
		return
	}

	// Notify the user the process has started
	fmt.Println("Hang tight while your environment bakes in the oven for a bit...")

	// List of environments.
	environments := []string{"SANDBOX_", "DEV_", "STAGE_", "PREPROD_", "PROD_"}

	// Use a wait group to wait for all goroutines to complete.
	var wg sync.WaitGroup

	// Create environment directories and copy the input directory into each concurrently.
	for _, env := range environments {
		envDirName := fmt.Sprintf("%s%s", env, filepath.Base(directoryName))
		envDirPath := filepath.Join(solutionPath, envDirName)

		err := os.MkdirAll(envDirPath, 0755)
		if err != nil {
			fmt.Printf("Failed to create environment directory '%s': %v\n", envDirName, err)
			continue
		}
		// Notify the user that we're still working.
		fmt.Printf("Still cooking... setting up %s\n", envDirName)

		// Increment the WaitGroup counter for each goroutine.
		wg.Add(1)

		// Copy the directory concurrently.
		go copyDirectory(directoryName, envDirPath, &wg)
	}

	// Wait for all goroutines to finish.
	wg.Wait()


fmt.Println("Environment setup completed successfully!")

// Ask if the user wants to create GitHub repositories for each environment
for _, env := range environments {
	envDirName := fmt.Sprintf("%s%s", env, filepath.Base(directoryName))
	if promptForGitRepo(envDirName) {
		// Create GitHub repository and set up git for each environment
		if err := createGitHubRepo(envDirName); err != nil {
			fmt.Printf("Error creating GitHub repository: %v\n", err)
			continue
		}

		// Set up git for the environment
		if err := setupGitForEnv(envDirName, solutionPath); err != nil {
			fmt.Printf("Error setting up git for %s: %v\n", envDirName, err)
		}
	}
}

}
