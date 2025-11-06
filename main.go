package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Config struct to hold our configuration
type Config struct {
	OrgFilepath        string   `json:"org_filepath"`
	OrgArchiveFilepath []string `json:"org_archive_filepath"`
}

var (
	config          Config
	configPath      string
	clipboardReader func() (string, error)
)

func init() {
	// Set up clipboard reader based on OS
	if commandExists("wl-paste") {
		clipboardReader = func() (string, error) {
			cmd := exec.Command("wl-paste")
			var out bytes.Buffer
			cmd.Stdout = &out
			err := cmd.Run()
			return strings.TrimSpace(out.String()), err
		}
	} else {
		clipboardReader = func() (string, error) {
			return "", fmt.Errorf("no clipboard command found (wl-paste)")
		}
	}
}

func main() {
	loadConfig()

	// Handle --help flag
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("ReadItLater - A tool to save URLs to an Org-mode file.")
		fmt.Println("Usage:")
		fmt.Println("  readitlater [URL]")
		fmt.Println("\nOptions:")
		fmt.Println("  --help, -h    Show this help message and exit.")
		fmt.Println("\nExamples:")
		fmt.Println("  # Pass a URL directly as an argument")
		fmt.Println("  readitlater https://example.com/article")
		fmt.Println("\n  # Save the URL from the clipboard")
		fmt.Println("  readitlater")
		os.Exit(0)
	}

	if _, err := os.Stat(config.OrgFilepath); os.IsNotExist(err) {
		notify("Warning", fmt.Sprintf("Could not find: %s", config.OrgFilepath))
		os.Exit(1)
	}

	var rawURL string
	if len(os.Args) > 1 {
		rawURL = os.Args[1]
	} else {
		var err error
		rawURL, err = clipboardReader()
		if err != nil {
			notify("Warning", "Could not get URL from clipboard")
			os.Exit(1)
		}
	}

	if !strings.HasPrefix(rawURL, "http") && !strings.HasPrefix(rawURL, "www") {
		notify("Warning", "Input is not a URL")
		os.Exit(1)
	}

	// Simple check for YouTube URL
	if strings.Contains(rawURL, "youtube.com") || strings.Contains(rawURL, "youtu.be") {
		processYouTube(rawURL)
	} else {
		processWebpage(rawURL)
	}
}

func loadConfig() {
	// Set default values to the XDG config directory
	configDir := filepath.Join(xdg.ConfigHome, "readitlater")
	config = Config{
		OrgFilepath:        filepath.Join(configDir, "ReadItLater.org"),
		OrgArchiveFilepath: []string{filepath.Join(configDir, "ReadItLater.org_archive"), filepath.Join(configDir, "Orgmode.org_archive")},
	}

	// Load from XDG config path
	var err error
	configPath, err = xdg.ConfigFile("readitlater/config.json")
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to get config path: %v", err))
		return
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Save default config if it doesn't exist
		saveConfig()
	} else {
		data, err := os.ReadFile(configPath)
		if err == nil {
			json.Unmarshal(data, &config)
		}
	}
}

func saveConfig() {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		notify("Error", fmt.Sprintf("Failed to create config directory: %v", err))
		return
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to marshal config: %v", err))
		return
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		notify("Error", fmt.Sprintf("Failed to write to file: %v", err))
	}
}

func notify(title, content string) {
	if commandExists("termux-notification") {
		cmd := exec.Command("termux-notification", "--title", title, "--content", content)
		cmd.Run()
	} else if commandExists("notify-send") {
		cmd := exec.Command("notify-send", "--app-name", "ReadItLater", "-i", "dialog-information", title, content)
		cmd.Run()
	} else {
		fmt.Printf("[%s] %s\n", title, content)
	}
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// getInput provides a unified way to get user input
func getInput(prompt string) (string, error) {
	var result string
	app := tview.NewApplication()
	form := tview.NewForm().
		SetFieldTextColor(tcell.ColorWhite).
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetButtonTextColor(tcell.ColorWhite).
		SetButtonBackgroundColor(tcell.ColorDefault)

	input := tview.NewInputField().SetLabel("Tag")
	form.AddFormItem(input)
	saveButton := tview.NewButton("Save")
	cancelButton := tview.NewButton("Cancel")

	// Set the background color for the selected state
	saveButton.SetBackgroundColorActivated(tcell.ColorGreen)
	cancelButton.SetBackgroundColorActivated(tcell.ColorRed)

	saveButton.SetSelectedFunc(func() {
		result = input.GetText()
		app.Stop()
	})

	cancelButton.SetSelectedFunc(func() {
		result = ""
		app.Stop()
	})

	form.AddButton("Save", func() {
		result = input.GetText()
		app.Stop()
	})
	form.AddButton("Cancel", func() {
		result = ""
		app.Stop()
	})

	form.SetBorder(true).SetTitle(prompt).SetTitleAlign(tview.AlignLeft)

	// Set maximum dimensions for the TUI
	const maxWidth = 80
	const maxHeight = 10

	// Create a flex container to center and limit the form dimensions
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, maxHeight, 1, true).
			AddItem(nil, 0, 1, false), maxWidth, 1, true).
		AddItem(nil, 0, 1, false)

	if err := app.SetRoot(flex, true).SetFocus(form).Run(); err != nil {
		return "", fmt.Errorf("tview application failed: %v", err)
	}
	if result == "" {
		return "", fmt.Errorf("user cancelled input")
	}
	return strings.TrimSpace(result), nil
}

func getTags() (string, error) {
	tags, err := getInput("Tags separated by ':'")
	if err != nil {
		return "", err
	}
	return tags + ":", nil
}

func getTitle() (string, error) {
	return getInput("Please, enter the Title manually")
}

func checkDuplicate(u string) error {
	files := append([]string{config.OrgFilepath}, config.OrgArchiveFilepath...)
	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		}
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), u) {
				notify("Warning", "Already in ReadItLater")
				return fmt.Errorf("duplicate URL found")
			}
		}
	}
	return nil
}

func processYouTube(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		notify("Error", "Invalid URL")
		return
	}
	// Remove query parameters except for the video ID
	query := u.Query()
	u.RawQuery = ""
	if v, ok := query["v"]; ok {
		u.RawQuery = "v=" + v[0]
	}

	url := u.String()

	if err := checkDuplicate(url); err != nil {
		return
	}

	customTags, err := getTags()
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to get tags: %v", err))
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to fetch YouTube page: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to read YouTube page body: %v", err))
		return
	}

	// Regex to extract title and duration
	titleRe := regexp.MustCompile(`<title>(.*?) - YouTube</title>`)
	durationRe := regexp.MustCompile(`"lengthSeconds":"(\d+)"`)

	titleMatch := titleRe.FindSubmatch(body)
	var title string
	if len(titleMatch) > 1 {
		title = string(titleMatch[1])
	}

	durationMatch := durationRe.FindSubmatch(body)
	var durationSeconds int
	if len(durationMatch) > 1 {
		durationSeconds, _ = strconv.Atoi(string(durationMatch[1]))
	}

	durationMinutes := durationSeconds / 60
	durationTag := "long"
	if durationMinutes <= 10 {
		durationTag = "short"
	} else if durationMinutes <= 30 {
		durationTag = "mid"
	}

	// Escape quotes and dashes
	title = strings.ReplaceAll(title, "\"", "")
	title = strings.ReplaceAll(title, "-", "")

	allTags := fmt.Sprintf(":%s:video:%s", durationTag, customTags)
	creationDate := time.Now().Format("2006-01-02")

	orgEntry := fmt.Sprintf(`* LATER %s %s
  :PROPERTIES:
  :CREATED: %s
  :LEN: %d
  :URL: %s
  :COMMENT:
  :END:
  %s

`, title, allTags, creationDate, durationMinutes, url, url)

	f, err := os.OpenFile(config.OrgFilepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to open file: %v", err))
		return
	}
	defer f.Close()
	if _, err := f.WriteString(orgEntry); err != nil {
		notify("Error", fmt.Sprintf("Failed to write to file: %v", err))
		return
	}

	notify("Info", fmt.Sprintf("[%d] %s (%s) saved", durationMinutes, title, allTags))
}

func processWebpage(rawURL string) {
	if err := checkDuplicate(rawURL); err != nil {
		return
	}

	customTags, err := getTags()
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to get tags: %v", err))
		return
	}

	// Fetch content
	resp, err := http.Get(rawURL)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to fetch webpage: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to read webpage body: %v", err))
		return
	}
	html := string(body)

	// Get title
	titleRe := regexp.MustCompile(`<title>(.*?)</title>`)
	titleMatch := titleRe.FindStringSubmatch(html)
	var title string
	if len(titleMatch) > 1 {
		title = titleMatch[1]
	} else {
		title, err = getTitle()
		if err != nil {
			notify("Error", "Could not get title automatically or manually")
			return
		}
	}

	// Use html2text for content processing
	// This requires the `html2text` command to be installed.
	cmd := exec.Command("html2text")
	cmd.Stdin = strings.NewReader(html)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		notify("Error", fmt.Sprintf("Failed to run html2text: %v", err))
		return
	}
	content := out.String()

	// Calculate reading time
	wordCount := len(strings.Fields(content))
	readingSpeed := 200 // words per minute
	readingTime := (wordCount / readingSpeed) + 1

	durationTag := "long"
	if readingTime <= 10 {
		durationTag = "short"
	} else if readingTime <= 30 {
		durationTag = "mid"
	}

	creationDate := time.Now().Format("2006-01-02")
	allTags := fmt.Sprintf(":%s:reading:%s", durationTag, customTags)

	orgEntry := fmt.Sprintf(`* TODO %s %s
  :PROPERTIES:
  :CREATED: %s
  :LEN: %d
  :URL: %s
  :COMMENT:
  :END:
  %s

`, title, allTags, creationDate, readingTime, rawURL, rawURL)

	f, err := os.OpenFile(config.OrgFilepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to open file: %v", err))
		return
	}
	defer f.Close()
	if _, err := f.WriteString(orgEntry); err != nil {
		notify("Error", fmt.Sprintf("Failed to write to file: %v", err))
		return
	}

	notify("Info", fmt.Sprintf("[%dm] %s (%s) saved", readingTime, title, allTags))
}
