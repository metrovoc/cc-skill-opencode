package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	shortHashLen = 6
	storeDir     = ".ocw"
	storeFile    = "sessions.json"
	serverPort   = "14200"

	// Model IDs — single source of truth. Bump these when upstream releases a new version.
	flashModel = "google-vertex/gemini-3-flash-preview"
	proModel   = "google-vertex/gemini-3.1-pro-preview"
	gptModel   = "openai/gpt-5.5"
)

var modelAliases = map[string]string{
	"flash": flashModel,
	"pro":   proModel,
	"gpt":   gptModel,
}

var modelVariants = map[string]string{
	"flash": "",     // default
	"pro":   "high",
	"gpt":   "xhigh",
}

// Tool permissions based on isolation (worktree or not)
var toolsReadOnly = map[string]bool{
	"question": false,
	"edit":     false,
	"write":    false,
}

var toolsFull = map[string]bool{
	"question": false, // Always disabled in headless mode
}

type Session struct {
	FullID    string    `json:"full_id"`
	Model     string    `json:"model"`
	ModelID   string    `json:"model_id"`
	Alias     string    `json:"alias"`
	Worktree  string    `json:"worktree,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	Sessions map[string]*Session `json:"sessions"`
	mu       sync.Mutex
	path     string
}

func getStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, storeDir, storeFile)
}

func loadStore() (*Store, error) {
	path := getStorePath()
	store := &Store{
		Sessions: make(map[string]*Session),
		path:     path,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &store.Sessions); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s.Sessions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) generateShortHash() string {
	for {
		bytes := make([]byte, 3)
		rand.Read(bytes)
		hash := hex.EncodeToString(bytes)
		if _, exists := s.Sessions[hash]; !exists {
			return hash
		}
	}
}

func (s *Store) findByPrefix(prefix string) (*Session, string, error) {
	var matches []string
	for hash := range s.Sessions {
		if strings.HasPrefix(hash, prefix) {
			matches = append(matches, hash)
		}
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("no session found: %s", prefix)
	}
	if len(matches) > 1 {
		return nil, "", fmt.Errorf("ambiguous: %s matches %v", prefix, matches)
	}

	return s.Sessions[matches[0]], matches[0], nil
}

// Server management
type Server struct {
	cmd     *exec.Cmd
	baseURL string
}

func startServer(cwd string) (*Server, error) {
	cmd := exec.Command("opencode", "serve", "--port", serverPort)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if cwd != "" {
		cmd.Dir = cwd
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	baseURL := "http://127.0.0.1:" + serverPort

	// Wait for server to be ready
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/global/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return &Server{cmd: cmd, baseURL: baseURL}, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	cmd.Process.Kill()
	return nil, fmt.Errorf("server failed to start")
}

func (s *Server) stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}

func (s *Server) createSession(title string) (string, error) {
	body, _ := json.Marshal(map[string]string{"title": title})
	resp, err := http.Post(s.baseURL+"/session", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// autoRejectPermissions listens to SSE and auto-rejects any permission requests
func (s *Server) autoRejectPermissions(ctx context.Context, sessionID string) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/event", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var event struct {
			Type       string `json:"type"`
			Properties struct {
				ID        string `json:"id"`
				SessionID string `json:"sessionID"`
			} `json:"properties"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// Only handle permission.asked events for our session
		if event.Type != "permission.asked" || event.Properties.SessionID != sessionID {
			continue
		}

		// Auto-reject the permission request
		rejectBody, _ := json.Marshal(map[string]interface{}{
			"response": "reject",
		})
		rejectReq, _ := http.NewRequestWithContext(ctx, "POST",
			fmt.Sprintf("%s/session/%s/permissions/%s", s.baseURL, sessionID, event.Properties.ID),
			bytes.NewReader(rejectBody))
		rejectReq.Header.Set("Content-Type", "application/json")
		rejectResp, err := http.DefaultClient.Do(rejectReq)
		if err == nil {
			rejectResp.Body.Close()
		}
	}
}

func (s *Server) sendMessage(sessionID, model, variant string, tools map[string]bool, text string, files []string, showThinking bool) (string, error) {
	// Parse model into provider/model
	modelParts := strings.SplitN(model, "/", 2)
	if len(modelParts) != 2 {
		return "", fmt.Errorf("invalid model format: %s", model)
	}

	// Build parts array
	var parts []map[string]interface{}

	// Add file parts first
	for _, f := range files {
		absPath, err := filepath.Abs(f)
		if err != nil {
			return "", fmt.Errorf("invalid file path %s: %w", f, err)
		}
		// Detect mime type (simplified - default to text/plain for unknown)
		mime := "text/plain"
		ext := strings.ToLower(filepath.Ext(f))
		switch ext {
		case ".pdf":
			mime = "application/pdf"
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".gif":
			mime = "image/gif"
		case ".webp":
			mime = "image/webp"
		case ".mp3":
			mime = "audio/mpeg"
		case ".mp4":
			mime = "video/mp4"
		case ".zip":
			mime = "application/zip"
		}
		parts = append(parts, map[string]interface{}{
			"type":     "file",
			"mime":     mime,
			"filename": filepath.Base(f),
			"url":      "file://" + absPath,
		})
	}

	// Add text part
	parts = append(parts, map[string]interface{}{
		"type": "text",
		"text": text,
	})

	modelConfig := map[string]string{
		"providerID": modelParts[0],
		"modelID":    modelParts[1],
	}
	if variant != "" {
		modelConfig["variant"] = variant
	}

	body, _ := json.Marshal(map[string]interface{}{
		"model": modelConfig,
		"tools": tools,
		"parts": parts,
	})

	// Start SSE listener to auto-reject permission requests
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.autoRejectPermissions(ctx, sessionID)

	resp, err := http.Post(
		fmt.Sprintf("%s/session/%s/message", s.baseURL, sessionID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode error: %w, body: %s", err, string(respBody))
	}

	var texts []string
	var thinking []string
	for _, p := range result.Parts {
		switch p.Type {
		case "text":
			texts = append(texts, p.Text)
		case "reasoning":
			if p.Text != "" {
				thinking = append(thinking, p.Text)
			}
		}
	}

	var output []string

	// Add thinking if requested
	if showThinking && len(thinking) > 0 {
		output = append(output, "<thinking>")
		output = append(output, thinking...)
		output = append(output, "</thinking>")
	}

	// Add text content
	output = append(output, texts...)

	return strings.Join(output, "\n"), nil
}

// Worktree management
func createWorktree(hash string) (string, error) {
	// Get git root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))

	// Create worktree at ../ocw-{hash}
	worktreePath := filepath.Join(filepath.Dir(repoRoot), "ocw-"+hash)
	branchName := "ocw-" + hash

	cmd = exec.Command("git", "worktree", "add", worktreePath, "-b", branchName)
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add failed: %s", string(output))
	}

	return worktreePath, nil
}

// Commands
func cmdNew(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ocw new <model> [--worktree]\nmodels: flash, pro, gpt")
	}

	alias := args[0]
	model, ok := modelAliases[alias]
	if !ok {
		return fmt.Errorf("unknown model: %s\navailable: flash, pro, gpt", alias)
	}

	useWorktree := false
	for _, arg := range args[1:] {
		if arg == "--worktree" || arg == "-w" {
			useWorktree = true
		}
	}

	store, err := loadStore()
	if err != nil {
		return fmt.Errorf("load store: %w", err)
	}

	shortHash := store.generateShortHash()

	// Create worktree first if requested
	var worktreePath string
	if useWorktree {
		worktreePath, err = createWorktree(shortHash)
		if err != nil {
			return fmt.Errorf("create worktree: %w", err)
		}
	}

	// Start temporary server (in worktree if creating worktree session)
	server, err := startServer(worktreePath)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer server.stop()

	// Create empty session
	fullID, err := server.createSession("ocw-" + shortHash)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	// Save to store
	store.Sessions[shortHash] = &Session{
		FullID:    fullID,
		Model:     model,
		ModelID:   model,
		Alias:     alias,
		Worktree:  worktreePath,
		CreatedAt: time.Now(),
	}

	if err := store.save(); err != nil {
		return fmt.Errorf("save store: %w", err)
	}

	// Output
	fmt.Println(shortHash)
	if worktreePath != "" {
		fmt.Println(worktreePath)
	}

	return nil
}

func cmdChat(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ocw chat <hash> [-f file...] [-t]\nprompt from stdin")
	}

	hashPrefix := args[0]
	var files []string
	showThinking := false

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-f":
			if i+1 < len(args) {
				files = append(files, args[i+1])
				i++
			}
		case "-t", "--thinking":
			showThinking = true
		}
	}

	store, err := loadStore()
	if err != nil {
		return fmt.Errorf("load store: %w", err)
	}

	session, _, err := store.findByPrefix(hashPrefix)
	if err != nil {
		return err
	}

	// Read prompt from stdin
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	promptStr := strings.TrimSpace(string(prompt))
	if promptStr == "" {
		return fmt.Errorf("empty prompt")
	}

	// Start temporary server (in worktree dir if session has worktree)
	server, err := startServer(session.Worktree)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer server.stop()

	// Get tool permissions based on isolation
	tools := toolsReadOnly
	if session.Worktree != "" {
		tools = toolsFull
	}

	// Get variant for this model
	variant := modelVariants[session.Alias]

	// Send message (with optional files)
	response, err := server.sendMessage(session.FullID, session.Model, variant, tools, promptStr, files, showThinking)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	fmt.Println(response)
	return nil
}

func cmdList(args []string) error {
	store, err := loadStore()
	if err != nil {
		return fmt.Errorf("load store: %w", err)
	}

	if len(store.Sessions) == 0 {
		fmt.Println("No sessions.")
		return nil
	}

	fmt.Printf("%-8s %-6s %-12s %s\n", "HASH", "MODEL", "AGE", "WORKTREE")
	fmt.Println(strings.Repeat("-", 60))

	for hash, sess := range store.Sessions {
		age := time.Since(sess.CreatedAt).Round(time.Minute)
		wt := "-"
		if sess.Worktree != "" {
			wt = sess.Worktree
		}
		fmt.Printf("%-8s %-6s %-12s %s\n", hash, sess.Alias, age.String(), wt)
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `ocw - OpenCode Wrapper

Commands:
  ocw new <model> [--worktree]   Create session (+ worktree if specified)
  ocw chat <hash> [-f file]      Send prompt (stdin) to session
  ocw list                       List sessions

Models: flash, pro, gpt

Examples:
  ocw new flash                  # Create flash session
  ocw new gpt --worktree         # Create gpt session with isolated worktree
  ocw chat abc123 <<< "hello"    # Chat with session
  ocw chat abc123 -f doc.pdf <<< "summarize"`)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(os.Args[2:])
	case "chat":
		err = cmdChat(os.Args[2:])
	case "list":
		err = cmdList(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command: %s", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
