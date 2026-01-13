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
	"golang.org/x/net/html"
)

// Config struct to hold our configuration
type Config struct {
	Backend            string   `json:"backend"` // "org" or "taskwarrior"
	OrgFilepath        string   `json:"org_filepath"`
	OrgArchiveFilepath []string `json:"org_archive_filepath"`
	TaskrcPath         string   `json:"taskrc_path"` // Path to taskrc file for taskwarrior backend
}

// Entry represents a processed URL with metadata
type Entry struct {
	Title        string
	URL          string
	Tags         []string
	Type         string // "video" or "reading"
	Duration     int    // in minutes
	DurationTag  string // "short", "mid", or "long"
	CreationDate string
}

// Backend interface for different storage backends
type Backend interface {
	SaveEntry(entry Entry) error
	CheckDuplicate(url string) error
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

// OrgBackend implements the Backend interface for Org-mode files
type OrgBackend struct {
	filepath        string
	archiveFilepath []string
}

func NewOrgBackend(filepath string, archiveFilepath []string) *OrgBackend {
	return &OrgBackend{
		filepath:        filepath,
		archiveFilepath: archiveFilepath,
	}
}

func (b *OrgBackend) CheckDuplicate(url string) error {
	files := append([]string{b.filepath}, b.archiveFilepath...)
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
			if strings.Contains(scanner.Text(), url) {
				notify("Warning", "Already in ReadItLater")
				return fmt.Errorf("duplicate URL found")
			}
		}
	}
	return nil
}

func (b *OrgBackend) SaveEntry(entry Entry) error {
	allTags := fmt.Sprintf(":%s:%s:", entry.DurationTag, entry.Type)
	for _, tag := range entry.Tags {
		if tag != "" {
			allTags += tag + ":"
		}
	}

	orgEntry := fmt.Sprintf(`* LATER %s %s
  :PROPERTIES:
  :CREATED: %s
  :LEN: %d
  :URL: %s
  :COMMENT:
  :END:
  %s

`, entry.Title, allTags, entry.CreationDate, entry.Duration, entry.URL, entry.URL)

	f, err := os.OpenFile(b.filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(orgEntry); err != nil {
		return fmt.Errorf("failed to write to file: %v", err)
	}

	return nil
}

// TaskwarriorBackend implements the Backend interface for Taskwarrior
type TaskwarriorBackend struct {
	taskrcPath string
}

func NewTaskwarriorBackend(taskrcPath string) *TaskwarriorBackend {
	return &TaskwarriorBackend{
		taskrcPath: taskrcPath,
	}
}

func (b *TaskwarriorBackend) CheckDuplicate(url string) error {
	// Query taskwarrior for tasks with matching URL
	args := []string{"rc:" + b.taskrcPath, "rc.verbose=nothing", "url:" + url, "count"}
	cmd := exec.Command("task", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to check duplicates in taskwarrior: %v", err)
	}

	count := strings.TrimSpace(out.String())
	if count != "0" {
		notify("Warning", "Already in ReadItLater")
		return fmt.Errorf("duplicate URL found")
	}

	return nil
}

func (b *TaskwarriorBackend) SaveEntry(entry Entry) error {
	// Build taskwarrior command with duration in description
	// Format: [Xm] Title for reading, [X] Title for videos (duration in minutes)
	description := fmt.Sprintf("[%dm] %s", entry.Duration, entry.Title)
	// Use subproject format: readitlater.short, readitlater.mid, or readitlater.long
	project := fmt.Sprintf("readitlater.%s", entry.DurationTag)
	args := []string{"rc:" + b.taskrcPath, "add", description, "project:" + project}

	// Add tags (content type and custom tags)
	args = append(args, "+"+entry.Type)
	for _, tag := range entry.Tags {
		if tag != "" {
			args = append(args, "+"+tag)
		}
	}

	// Add custom UDA field (only url)
	args = append(args, "url:"+entry.URL)

	cmd := exec.Command("task", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add task to taskwarrior: %v, stderr: %s", err, stderr.String())
	}

	return nil
}

func getBackend() Backend {
	switch config.Backend {
	case "taskwarrior":
		return NewTaskwarriorBackend(config.TaskrcPath)
	case "org":
		fallthrough
	default:
		return NewOrgBackend(config.OrgFilepath, config.OrgArchiveFilepath)
	}
}

func main() {
	loadConfig()

	// Handle --help flag
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("ReadItLater - A tool to save URLs to an Org-mode file or Taskwarrior.")
		fmt.Println("Usage:")
		fmt.Println("  readitlater [URL]")
		fmt.Println("\nOptions:")
		fmt.Println("  --help, -h    Show this help message and exit.")
		fmt.Println("\nExamples:")
		fmt.Println("  # Pass a URL directly as an argument")
		fmt.Println("  readitlater https://example.com/article")
		fmt.Println("\n  # Save the URL from the clipboard")
		fmt.Println("  readitlater")
		fmt.Println("\nConfiguration:")
		fmt.Println("  Edit ~/.config/readitlater/config.json to configure backend (org or taskwarrior)")
		fmt.Println("\nTaskwarrior Setup:")
		fmt.Println("  For taskwarrior backend, add this UDA to ~/.taskrc:")
		fmt.Println("    uda.url.type=string")
		fmt.Println("    uda.url.label=URL")
		fmt.Println("  Duration is included in the task description as [Xm]")
		fmt.Println("  Duration categories (short/mid/long) are set as subprojects: readitlater.short, readitlater.mid, readitlater.long")
		fmt.Println("  Tags include: reading/video, and your custom tags")
		os.Exit(0)
	}

	// Validate backend requirements
	if config.Backend == "org" {
		if _, err := os.Stat(config.OrgFilepath); os.IsNotExist(err) {
			notify("Warning", fmt.Sprintf("Could not find: %s", config.OrgFilepath))
			os.Exit(1)
		}
	} else if config.Backend == "taskwarrior" {
		if !commandExists("task") {
			notify("Error", "Taskwarrior (task) command not found. Please install taskwarrior.")
			os.Exit(1)
		}
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
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "~"
	}
	config = Config{
		Backend:            "org",
		OrgFilepath:        filepath.Join(configDir, "ReadItLater.org"),
		OrgArchiveFilepath: []string{filepath.Join(configDir, "ReadItLater.org_archive"), filepath.Join(configDir, "Orgmode.org_archive")},
		TaskrcPath:         filepath.Join(homeDir, ".taskrc"),
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

	// Get terminal dimensions and calculate responsive max dimensions
	screen, err := tcell.NewScreen()
	var maxWidth, maxHeight int
	if err == nil {
		if initErr := screen.Init(); initErr == nil {
			termWidth, termHeight := screen.Size()
			screen.Fini()

			// Use 90% of terminal width, but cap at 80 chars and ensure minimum of 30
			maxWidth = min(80, termWidth*9/10)
			if maxWidth < 30 {
				maxWidth = min(30, termWidth)
			}

			// Use 80% of terminal height, but cap at 10 lines and ensure minimum of 8
			maxHeight = min(10, termHeight*8/10)
			if maxHeight < 8 {
				maxHeight = min(8, termHeight)
			}
		} else {
			// Fallback to reasonable defaults if screen init fails
			maxWidth = 60
			maxHeight = 8
		}
	} else {
		// Fallback to reasonable defaults if screen creation fails
		maxWidth = 60
		maxHeight = 8
	}

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

func processYouTube(rawURL string) {
	backend := getBackend()

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

	if err := backend.CheckDuplicate(url); err != nil {
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

	// Parse custom tags
	var tagList []string
	if customTags != "" {
		customTags = strings.Trim(customTags, ":")
		if customTags != "" {
			tagList = strings.Split(customTags, ":")
		}
	}

	entry := Entry{
		Title:        title,
		URL:          url,
		Tags:         tagList,
		Type:         "video",
		Duration:     durationMinutes,
		DurationTag:  durationTag,
		CreationDate: time.Now().Format("2006-01-02"),
	}

	if err := backend.SaveEntry(entry); err != nil {
		notify("Error", fmt.Sprintf("Failed to save entry: %v", err))
		return
	}

	notify("Info", fmt.Sprintf("[%d] %s saved", durationMinutes, title))
}

// extractTextFromHTML converts HTML to plain text by extracting text content from HTML nodes
func extractTextFromHTML(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var textBuilder strings.Builder
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				textBuilder.WriteString(text)
				textBuilder.WriteString(" ")
			}
		}
		// Skip script and style tags
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	return textBuilder.String(), nil
}

func processWebpage(rawURL string) {
	backend := getBackend()

	if err := backend.CheckDuplicate(rawURL); err != nil {
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

	// Use helper function to convert HTML to text for word count estimation
	content, err := extractTextFromHTML(html)
	if err != nil {
		notify("Error", fmt.Sprintf("Failed to convert HTML to text: %v", err))
		return
	}

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

	// Parse custom tags
	var tagList []string
	if customTags != "" {
		customTags = strings.Trim(customTags, ":")
		if customTags != "" {
			tagList = strings.Split(customTags, ":")
		}
	}

	entry := Entry{
		Title:        title,
		URL:          rawURL,
		Tags:         tagList,
		Type:         "reading",
		Duration:     readingTime,
		DurationTag:  durationTag,
		CreationDate: time.Now().Format("2006-01-02"),
	}

	if err := backend.SaveEntry(entry); err != nil {
		notify("Error", fmt.Sprintf("Failed to save entry: %v", err))
		return
	}

	notify("Info", fmt.Sprintf("[%dm] %s saved", readingTime, title))
}
