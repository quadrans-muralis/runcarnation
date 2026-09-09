package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// pollInterval is how often runcarnation checks GitHub for new commits
// while the application is running.
const pollInterval = 5 * time.Second

// logRetention is how long a log file is kept before cleanOldLogs removes it.
const logRetention = 14 * 24 * time.Hour

// webListenAddr is where the embedded web console listens.
const webListenAddr = "0.0.0.0:8520"

// webBrowserURL is what gets opened in the user's default browser on startup.
const webBrowserURL = "http://localhost:8520/"

type Language string

const (
	LangEN Language = "en"
	LangJP Language = "jp"
	LangCN Language = "cn"
)

var currentLanguage = LangEN

var translations = map[Language]map[string]string{

	LangEN: {
		"command_center": "COMMAND CENTER",

		"check":        "Scan the repository and compare it with GitHub.",
		"status":       "Inspect the local working tree.",
		"remote":       "Display the GitHub remote connection.",
		"fetch":        "Retrieve the latest information from GitHub.",
		"diff":         "Analyze differences between local and GitHub.",
		"ignored":      "List files matched by .gitignore (absolute paths).",
		"changed":      "List files with uncommitted changes (absolute paths).",
		"run":          "Sync with GitHub, then run the application once.",
		"runcarnation": "Run the application, watching GitHub and auto-restarting on updates.",
		"demolition":   "Clean out the stored build binaries.",
		"help":         "Display this command center.",
		"exit":         "Shut down GO RUNCARNATION.",

		"language": "Change interface language.",

		"err_path_empty":    "Path cannot be empty.",
		"err_path_notfound": "Path does not exist:",
		"err_path_notdir":   "Path is not a directory:",
	},

	LangJP: {
		"command_center": "COMMAND CENTER",

		"check":        "リポジトリをスキャンしてGitHubと比較します。",
		"status":       "ローカルの作業ツリーを確認します。",
		"remote":       "GitHubとの接続先を表示します。",
		"fetch":        "GitHubから最新情報を取得します。",
		"diff":         "ローカルとGitHubの差分を解析します。",
		"ignored":      ".gitignoreで無視されているファイルの絶対パス一覧を表示します。",
		"changed":      "変更のあるファイルの絶対パス一覧を表示します。",
		"run":          "GitHubと同期してから、アプリケーションを1回だけ実行します。",
		"runcarnation": "アプリケーションを実行し、GitHubを監視して更新があれば自動的に再起動を繰り返します。",
		"demolition":   "保存されているビルド済みバイナリを消去します。",
		"help":         "コマンドセンターを表示します。",
		"exit":         "GO RUNCARNATIONを終了します。",

		"language": "インターフェースの言語を変更します。",

		"err_path_empty":    "パスを空にすることはできません。",
		"err_path_notfound": "パスが存在しません:",
		"err_path_notdir":   "パスがディレクトリではありません:",
	},

	LangCN: {
		"command_center": "COMMAND CENTER",

		"check":        "扫描仓库并与 GitHub 进行比较。",
		"status":       "检查本地工作区状态。",
		"remote":       "显示 GitHub 远程仓库连接。",
		"fetch":        "从 GitHub 获取最新信息。",
		"diff":         "分析本地仓库与 GitHub 之间的差异。",
		"ignored":      "列出被 .gitignore 忽略的文件（绝对路径）。",
		"changed":      "列出有未提交更改的文件（绝对路径）。",
		"run":          "先与 GitHub 同步，然后运行一次应用程序。",
		"runcarnation": "运行应用程序，监视 GitHub 并在有更新时自动反复重启。",
		"demolition":   "清理已保存的构建二进制文件。",
		"help":         "显示命令中心。",
		"exit":         "关闭 GO RUNCARNATION。",

		"language": "更改界面语言。",

		"err_path_empty":    "路径不能为空。",
		"err_path_notfound": "路径不存在:",
		"err_path_notdir":   "路径不是一个目录:",
	},
}

func t(key string) string {

	lang, ok := translations[currentLanguage]

	if !ok {
		lang = translations[LangEN]
	}

	text, ok := lang[key]

	if !ok {
		// Fall back to English if the key is missing.
		return translations[LangEN][key]
	}

	return text
}

var commandAliases = map[string]string{
	"chk":  "check",
	"stat": "status",
	"rmt":  "remote",
	"fch":  "fetch",
	"ign":  "ignored",
	"chg":  "changed",
	"rcn":  "runcarnation",
	"demo": "demolition",
	"lang": "language",
}

func normalizeCommand(cmd string) string {
	if canonical, ok := commandAliases[cmd]; ok {
		return canonical
	}
	return cmd
}

func runGit(path string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", path}, args...)

	cmd := exec.Command("git", cmdArgs...)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func isGitRepository(path string) bool {
	_, err := runGit(path, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func getBranch(path string) (string, error) {
	return runGit(path, "branch", "--show-current")
}

func getRemote(path string) (string, error) {
	return runGit(path, "remote", "get-url", "origin")
}

func validatePath(path string) error {
	info, err := os.Stat(path)

	if err != nil {
		return fmt.Errorf("%s %s", t("err_path_notfound"), path)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s %s", t("err_path_notdir"), path)
	}

	return nil
}

func fetch(path string) error {
	cmd := exec.Command("git", "-C", path, "fetch", "origin")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func compare(path string, branch string) (int, int, error) {
	output, err := runGit(
		path,
		"rev-list",
		"--left-right",
		"--count",
		"HEAD...origin/"+branch,
	)

	if err != nil {
		return 0, 0, err
	}

	var ahead, behind int

	_, err = fmt.Sscanf(output, "%d %d", &ahead, &behind)

	if err != nil {
		return 0, 0, err
	}

	return ahead, behind, nil
}

var buildDirName = filepath.Join("GO_RUNCARNATION", "runcarnation")

func getBuildDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine config directory: %w", err)
	}

	dir := filepath.Join(base, buildDirName)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create build directory %s: %w", dir, err)
	}

	return dir, nil
}

func buildBinary(path string) (string, error) {
	dir, err := getBuildDir()
	if err != nil {
		return "", err
	}

	binName := fmt.Sprintf("go-runcarnation-app-%d-%d", os.Getpid(), time.Now().UnixNano())
	binPath := filepath.Join(dir, binName)

	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	fmt.Println("[BUILD] Compiling application into", dir)

	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}

	return binPath, nil
}

var logDirName = filepath.Join("GO_RUNCARNATION", "log")

func getLogDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine config directory: %w", err)
	}

	dir := filepath.Join(base, logDirName)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create log directory %s: %w", dir, err)
	}

	return dir, nil
}

func cleanOldLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-logRetention)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func setupLogging() (cleanup func(), err error) {
	dir, err := getLogDir()
	if err != nil {
		return nil, err
	}

	cleanOldLogs(dir)

	logPath := filepath.Join(dir, fmt.Sprintf("run-%s.log", time.Now().Format("20060102-150405")))

	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("could not create log file %s: %w", logPath, err)
	}

	origStdout := os.Stdout
	origStderr := os.Stderr

	r, w, err := os.Pipe()
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("could not create logging pipe: %w", err)
	}

	os.Stdout = w
	os.Stderr = w

	done := make(chan struct{})

	go func() {
		defer close(done)
		_, _ = io.Copy(io.MultiWriter(origStdout, logFile), r)
	}()

	cleanup = func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
		w.Close()
		<-done
		logFile.Close()
	}

	return cleanup, nil
}

func startBinary(binPath string, workDir string) (*exec.Cmd, error) {
	cmd := exec.Command(binPath)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start application: %w", err)
	}

	return cmd, nil
}

func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

func notifyRestart(message string) {
	const title = "GO RUNCARNATION"

	switch runtime.GOOS {

	case "windows":
		script := fmt.Sprintf(
			"[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null; "+
				"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType = WindowsRuntime] > $null; "+
				"$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02); "+
				"$text = $template.GetElementsByTagName('text'); "+
				"$text.Item(0).AppendChild($template.CreateTextNode('%s')) > $null; "+
				"$text.Item(1).AppendChild($template.CreateTextNode('%s')) > $null; "+
				"$toast = [Windows.UI.Notifications.ToastNotification]::new($template); "+
				"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('GO RUNCARNATION').Show($toast);",
			psEscape(title), psEscape(message),
		)
		runNotifyCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", script)

	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", message, title)
		runNotifyCommand("osascript", "-e", script)

	case "linux":
		runNotifyCommand("notify-send", title, message)

	default:
		// No known notifier for this platform; skip silently.
	}
}

func psEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func runNotifyCommand(name string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = exec.CommandContext(ctx, name, args...).Run()
}

func banner() {
	fmt.Println()
	fmt.Print(`
 ______  _____        ______ _     _ __   _ _______ _______  ______ __   _ _______ _______ _____  _____  __   _
|  ____ |     |      |_____/ |     | | \  | |       |_____| |_____/ | \  | |_____|    |      |   |     | | \  |
|_____| |_____|      |    \_ |_____| |  \_| |_____  |     | |    \_ |  \_| |     |    |    __|__ |_____| |  \_|
`)
	fmt.Println()
	fmt.Println("G I T H U B   R E P O   S C A N N E R")
	fmt.Println("            Version 1.0.0")
	fmt.Println("--------------------------------------")
	fmt.Println()
}

func section(title string) {
	fmt.Println()
	fmt.Println("┌──────────────────────────────────────────────┐")
	fmt.Printf("│  %-44s│\n", title)
	fmt.Println("└──────────────────────────────────────────────┘")
	fmt.Println()
}

func status(path string) {
	section("WORKING TREE SCAN")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if !isGitRepository(path) {
		fmt.Println("[ERROR] This directory is not a Git repository.")
		return
	}

	output, err := runGit(path, "status", "--short")

	if err != nil {
		fmt.Println("[ERROR] Failed to inspect working tree.")
		return
	}

	if output == "" {
		fmt.Println("[OK]     Working tree is clean.")
		fmt.Println("        No uncommitted changes detected.")
	} else {
		fmt.Println("[WARN]   Uncommitted changes detected!")
		fmt.Println()
		fmt.Println(output)
	}
}

func remote(path string) {
	section("REMOTE LINK")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	output, err := getRemote(path)

	if err != nil {
		fmt.Println("[ERROR] No 'origin' remote is configured.")
		return
	}

	fmt.Println("[LINK]   origin")
	fmt.Println("        " + output)
}

func check(path string) {
	section("REPOSITORY SCAN")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if !isGitRepository(path) {
		fmt.Println("[ERROR] Target is not a Git repository.")
		return
	}

	branch, err := getBranch(path)

	if err != nil {
		fmt.Println("[ERROR] Failed to determine current branch.")
		return
	}

	if branch == "" {
		fmt.Println("[ERROR] Repository is in a detached HEAD state; no branch to compare.")
		return
	}

	remoteURL, err := getRemote(path)

	if err != nil {
		fmt.Println("[ERROR] No 'origin' remote is configured.")
		return
	}

	fmt.Println("Target")
	fmt.Println("  Path       :", path)
	fmt.Println("  Repository :", remoteURL)
	fmt.Println("  Branch     :", branch)

	fmt.Println()
	fmt.Println("[SCAN] Connecting to GitHub...")
	fmt.Println()

	if err := fetch(path); err != nil {
		fmt.Println("[ERROR] Unable to fetch from GitHub.")
		return
	}

	fmt.Println()
	fmt.Println("[OK] GitHub data received.")
	fmt.Println()

	ahead, behind, err := compare(path, branch)

	if err != nil {
		fmt.Println("[ERROR] Failed to compare repository state.")
		return
	}

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║              REPOSITORY STATUS               ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	switch {

	case ahead == 0 && behind == 0:
		fmt.Println("  [✓] STATUS: UP TO DATE")
		fmt.Println()
		fmt.Println("      Your local repository matches GitHub.")
		fmt.Println("      Nothing to synchronize.")

	case ahead == 0 && behind > 0:
		fmt.Println("  [↓] STATUS: OUTDATED")
		fmt.Println()
		fmt.Printf("      GitHub is %d commit(s) ahead.\n", behind)
		fmt.Println("      Your local repository needs an update.")
		fmt.Println()
		fmt.Println("      Hint: git pull")

	case ahead > 0 && behind == 0:
		fmt.Println("  [↑] STATUS: LOCAL AHEAD")
		fmt.Println()
		fmt.Printf("      Your local repository is %d commit(s) ahead.\n", ahead)
		fmt.Println("      These changes have not been pushed to GitHub.")
		fmt.Println()
		fmt.Println("      Hint: git push")

	default:
		fmt.Println("  [!] STATUS: DIVERGED")
		fmt.Println()
		fmt.Println("      Local and GitHub have different histories.")
		fmt.Println()
		fmt.Printf("      Local only  : %d commit(s)\n", ahead)
		fmt.Printf("      GitHub only : %d commit(s)\n", behind)
		fmt.Println()
		fmt.Println("      Manual intervention may be required.")
	}

	fmt.Println()
}

func diff(path string) {
	section("DIVERGENCE ANALYSIS")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}
	if !isGitRepository(path) {
		fmt.Println("[ERROR] Target is not a Git repository.")
		return
	}

	branch, err := getBranch(path)
	if err != nil {
		fmt.Println("[ERROR] Failed to determine branch.")
		return
	}
	if branch == "" {
		fmt.Println("[ERROR] Repository is in a detached HEAD state; no branch to compare.")
		return
	}

	fmt.Println("[SCAN] Connecting to GitHub...")
	if err := fetch(path); err != nil {
		fmt.Println("[ERROR] Unable to fetch from GitHub.")
		return
	}

	fmt.Println("[SCAN] Comparing local HEAD with origin/" + branch)
	fmt.Println()

	output, err := runGit(

		path,

		"diff",

		"--stat",

		"HEAD...origin/"+branch,
	)

	if err != nil {

		fmt.Println("[ERROR] Unable to generate diff.")

		return

	}

	if output == "" {

		fmt.Println("[OK] No differences detected.")

		return

	}

	fmt.Println(output)

}

func toAbsolutePath(repoPath, relPath string) string {
	abs, err := filepath.Abs(filepath.Join(repoPath, relPath))
	if err != nil {
		return relPath
	}
	return abs
}

func ignored(path string) {
	section("IGNORED FILES")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if !isGitRepository(path) {
		fmt.Println("[ERROR] Target is not a Git repository.")
		return
	}

	output, err := runGit(path, "ls-files", "--others", "--ignored", "--exclude-standard")
	if err != nil {
		fmt.Println("[ERROR] Failed to list ignored files.")
		return
	}

	if output == "" {
		fmt.Println("[OK] No ignored files found.")
		return
	}

	lines := strings.Split(output, "\n")

	fmt.Printf("[INFO] %d ignored file(s):\n\n", len(lines))

	for _, rel := range lines {
		fmt.Println("  " + toAbsolutePath(path, rel))
	}
}

func changed(path string) {
	section("CHANGED FILES")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if !isGitRepository(path) {
		fmt.Println("[ERROR] Target is not a Git repository.")
		return
	}

	output, err := runGit(path, "status", "--porcelain")
	if err != nil {
		fmt.Println("[ERROR] Failed to inspect working tree.")
		return
	}

	if output == "" {
		fmt.Println("[OK] No changes detected.")
		return
	}

	lines := strings.Split(output, "\n")

	fmt.Printf("[INFO] %d changed file(s):\n\n", len(lines))

	for _, line := range lines {
		if len(line) < 4 {
			continue
		}

		rel := strings.TrimSpace(line[3:])

		// Renames are reported as "old -> new"; only the new path matters here.
		if idx := strings.Index(rel, " -> "); idx != -1 {
			rel = rel[idx+len(" -> "):]
		}

		fmt.Println("  " + toAbsolutePath(path, rel))
	}
}

func help() {
	section(t("command_center"))

	fmt.Println("  check (chk)")
	fmt.Println("      " + t("check"))
	fmt.Println()

	fmt.Println("  status (stat)")
	fmt.Println("      " + t("status"))
	fmt.Println()

	fmt.Println("  remote (rmt)")
	fmt.Println("      " + t("remote"))
	fmt.Println()

	fmt.Println("  fetch (fch)")
	fmt.Println("      " + t("fetch"))
	fmt.Println()

	fmt.Println("  diff")
	fmt.Println("      " + t("diff"))
	fmt.Println()

	fmt.Println("  ignored (ign)")
	fmt.Println("      " + t("ignored"))
	fmt.Println()

	fmt.Println("  changed (chg)")
	fmt.Println("      " + t("changed"))
	fmt.Println()

	fmt.Println("  run")
	fmt.Println("      " + t("run"))
	fmt.Println()

	fmt.Println("  runcarnation (rcn)")
	fmt.Println("      " + t("runcarnation"))
	fmt.Println()

	fmt.Println("  demolition (demo)")
	fmt.Println("      " + t("demolition"))
	fmt.Println()

	fmt.Println("  help")
	fmt.Println("      " + t("help"))
	fmt.Println()

	fmt.Println("  language (lang)")
	fmt.Println("      " + t("language"))
	fmt.Println()

	fmt.Println("  exit")
	fmt.Println("      " + t("exit"))
	fmt.Println()
}

func applyLanguage(code string) bool {
	switch code {
	case "en":
		currentLanguage = LangEN
	case "jp":
		currentLanguage = LangJP
	case "cn":
		currentLanguage = LangCN
	default:
		return false
	}
	return true
}

func language() {
	section("LANGUAGE")

	fmt.Println("  en. English")
	fmt.Println("  jp. 日本語")
	fmt.Println("  cn. 中文")
	fmt.Println()

	fmt.Print("Select language: ")

	scanner := bufio.NewScanner(os.Stdin)

	if !scanner.Scan() {
		return
	}

	code := strings.TrimSpace(scanner.Text())

	if !applyLanguage(code) {
		fmt.Println("Invalid selection.")
		fmt.Println()
		return
	}

	switch code {
	case "en":
		fmt.Println("Language changed to English.")
	case "jp":
		fmt.Println("言語を日本語に変更しました。")
	case "cn":
		fmt.Println("语言已切换为中文。")
	}

	fmt.Println()
}

func gitPull(path string) error {
	fmt.Println("[UPDATE] Pulling latest changes from GitHub...")
	_, err := runGit(path, "pull")
	return err
}

func syncToLatest(path, branch string) (pulled int, err error) {
	if err := fetch(path); err != nil {
		return 0, fmt.Errorf("fetch failed: %w", err)
	}

	_, behind, err := compare(path, branch)
	if err != nil {
		return 0, fmt.Errorf("comparison failed: %w", err)
	}

	if behind == 0 {
		return 0, nil
	}

	if err := gitPull(path); err != nil {
		return 0, fmt.Errorf("update failed: %w", err)
	}

	return behind, nil
}

func runOnce(path string) {
	section("RUN")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if !isGitRepository(path) {
		fmt.Println("[ERROR] Target is not a Git repository.")
		return
	}

	branch, err := getBranch(path)
	if err != nil {
		fmt.Println("[ERROR] Failed to determine branch.")
		return
	}

	if branch == "" {
		fmt.Println("[ERROR] Repository is in a detached HEAD state; nothing to sync.")
		return
	}

	fmt.Println("[SCAN] Syncing with GitHub before launch...")

	pulled, err := syncToLatest(path, branch)
	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if pulled > 0 {
		fmt.Printf("[UPDATE] Pulled %d new commit(s).\n", pulled)
	} else {
		fmt.Println("[OK] Already up to date.")
	}

	fmt.Println()

	binPath, err := buildBinary(path)
	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}
	defer os.Remove(binPath)

	fmt.Println("[RUN] Starting the application:", binPath)

	cmd, err := startBinary(binPath, path)
	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("[ERROR] Application exited with error: %v\n", err)
	} else {
		fmt.Println("[OK] Application exited normally.")
	}
}

func runcarnation(path string) {
	section("RUNCARNATION")

	if err := validatePath(path); err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if !isGitRepository(path) {
		fmt.Println("[ERROR] Target is not a Git repository.")
		return
	}

	branch, err := getBranch(path)
	if err != nil {
		fmt.Println("[ERROR] Failed to determine branch.")
		return
	}

	if branch == "" {
		fmt.Println("[ERROR] Repository is in a detached HEAD state; nothing to track.")
		return
	}

	fmt.Printf("[MONITOR] Watching branch %q for updates (checking every %s).\n", branch, pollInterval)
	fmt.Println("[MONITOR] The application restarts automatically when new commits arrive.")
	fmt.Println("[MONITOR] If the application ends on its own, the cycle stops there.")

	for {
		fmt.Println()
		fmt.Println("[SCAN] Syncing with GitHub before launch...")

		pulled, err := syncToLatest(path, branch)
		if err != nil {
			fmt.Println("[ERROR]", err)
			return
		}

		if pulled > 0 {
			fmt.Printf("[UPDATE] Pulled %d new commit(s).\n", pulled)
		} else {
			fmt.Println("[OK] Already up to date.")
		}

		binPath, err := buildBinary(path)
		if err != nil {
			fmt.Println("[ERROR]", err)
			return
		}

		restart, err := runOnceUntilUpdateOrExit(binPath, path, branch)

		os.Remove(binPath)

		if err != nil {
			fmt.Println("[ERROR]", err)
			return
		}

		if !restart {
			break
		}
	}

	fmt.Println()
	fmt.Println("[MONITOR] Stopped.")
}

func runOnceUntilUpdateOrExit(binPath, workDir, branch string) (restart bool, err error) {
	cmd, err := startBinary(binPath, workDir)
	if err != nil {
		return false, err
	}

	fmt.Printf("[RUN] Application started (pid %d): %s\n", cmd.Process.Pid, binPath)

	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
	}()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {

		case waitErr := <-exited:
			if waitErr != nil {
				fmt.Println("[INFO] Application exited with an error:", waitErr)
			} else {
				fmt.Println("[INFO] Application exited normally.")
			}
			return false, nil

		case <-ticker.C:
			if err := fetch(workDir); err != nil {
				// A transient network hiccup shouldn't kill the running app.
				fmt.Println("[WARN] Could not reach GitHub; will retry.")
				continue
			}

			_, behind, err := compare(workDir, branch)
			if err != nil {
				fmt.Println("[WARN] Comparison failed; will retry.")
				continue
			}

			if behind == 0 {
				continue
			}

			fmt.Printf("\n[UPDATE] %d new commit(s) detected on GitHub. Restarting application...\n", behind)

			notifyRestart(fmt.Sprintf("%d new commit(s) detected. Restarting the application.", behind))

			killProcess(cmd)

			<-exited

			return true, nil
		}
	}
}

func demolition() {
	section("DEMOLITION")

	dir, err := getBuildDir()
	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("[ERROR] Failed to read build directory:", err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("[OK] Build directory is already empty.")
		fmt.Println("     " + dir)
		return
	}

	removed := 0

	for _, entry := range entries {
		target := filepath.Join(dir, entry.Name())

		if err := os.RemoveAll(target); err != nil {
			fmt.Println("[WARN] Failed to remove:", target, "-", err)
			continue
		}

		removed++
	}

	fmt.Printf("[OK] Removed %d item(s) from %s\n", removed, dir)
}

func readRepoPath(scanner *bufio.Scanner) (path string, eof bool) {
	fmt.Print("Repository path: ")

	if !scanner.Scan() {
		return "", true
	}

	return strings.TrimSpace(scanner.Text()), false
}

// doFetch performs a plain "git fetch origin" and prints a short summary.
// Shared between the interactive "fetch" command and non-interactive mode.
func doFetch(path string) {
	fmt.Println()
	fmt.Println("[SCAN] Contacting GitHub...")

	if err := fetch(path); err != nil {
		fmt.Println("[ERROR] Fetch failed.")
	} else {
		fmt.Println("[OK]    Repository intelligence updated.")
	}

	fmt.Println()
}

func runNonInteractive(args []string) int {
	cmd := normalizeCommand(args[0])

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	path := fs.String("path", "", "Target repository path")
	lang := fs.String("lang", "", "Interface language: en, jp, or cn")

	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	if *lang != "" && !applyLanguage(*lang) {
		fmt.Println("[ERROR] Unknown language:", *lang)
		return 1
	}

	switch cmd {

	case "help":
		help()
		return 0

	case "demolition":
		demolition()
		return 0

	case "language":
		fmt.Println("Current language:", currentLanguage)
		return 0

	case "check", "status", "remote", "fetch", "diff", "ignored", "changed", "run", "runcarnation":
		if *path == "" {
			fmt.Printf("[ERROR] --path is required for the %q command in non-interactive mode.\n", cmd)
			return 1
		}

		switch cmd {
		case "check":
			check(*path)
		case "status":
			status(*path)
		case "remote":
			remote(*path)
		case "fetch":
			doFetch(*path)
		case "diff":
			diff(*path)
		case "ignored":
			ignored(*path)
		case "changed":
			changed(*path)
		case "run":
			runOnce(*path)
		case "runcarnation":
			runcarnation(*path)
		}

		return 0

	default:
		fmt.Println("[UNKNOWN] Command not recognized:", cmd)
		return 1
	}
}

// runShellSession runs the interactive command loop. It reads from os.Stdin
// and writes to os.Stdout/os.Stderr, whatever those currently point to - this
// lets the same logic power both the local terminal and the web console,
// which temporarily redirect these to pipes for the duration of a session.
func runShellSession() {
	scanner := bufio.NewScanner(os.Stdin)

ShellLoop:
	for {
		fmt.Print("GO RUNCARNATION> ")

		if !scanner.Scan() {
			break ShellLoop
		}

		input := normalizeCommand(strings.TrimSpace(scanner.Text()))

		switch input {

		case "help":
			help()

		case "check", "status", "remote", "fetch", "diff", "ignored", "changed", "run", "runcarnation":
			path, eof := readRepoPath(scanner)

			if eof {
				break ShellLoop
			}

			if path == "" {
				fmt.Println("[ERROR]", t("err_path_empty"))
				continue ShellLoop
			}

			switch input {

			case "check":
				check(path)

			case "status":
				status(path)

			case "remote":
				remote(path)

			case "fetch":
				doFetch(path)

			case "diff":
				diff(path)

			case "ignored":
				ignored(path)

			case "changed":
				changed(path)

			case "run":
				runOnce(path)

			case "runcarnation":
				runcarnation(path)
			}

		case "language":
			language()

		case "demolition":
			demolition()

		case "exit":
			fmt.Println()
			fmt.Println("Connection terminated.")
			fmt.Println("Goodbye, operator.")
			fmt.Println()
			return

		case "":
			// Ignore empty input.

		default:
			fmt.Println()
			fmt.Println("[UNKNOWN] Command not recognized:", input)
			fmt.Println("          Type 'help' for available commands.")
			fmt.Println()
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("[ERROR] Input error:", err)
	}
}

// ---------------------------------------------------------------------------
// Web console
//
// This exposes the exact same interactive shell (runShellSession) through a
// browser-based terminal served at http://0.0.0.0:8520/. Output is streamed
// to the browser over Server-Sent Events; keystrokes typed into the page are
// POSTed back and fed into the shell's stdin.
//
// The shell session's lifetime is independent of any single HTTP connection:
// if a browser tab is closed, the network drops, or the page is reloaded,
// the underlying shell (and any command it's running, e.g. "runcarnation")
// keeps going untouched. Reconnecting (or opening a new tab) re-attaches to
// the same running session and replays what was missed. The ONLY way the
// session itself ends is the operator explicitly typing "exit".
// ---------------------------------------------------------------------------

// outputHub fans a session's output out to any number of currently attached
// SSE viewers, and keeps a bounded backlog so a viewer that (re)connects
// mid-session can catch up on what it missed.
type outputHub struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
	history     []byte
}

const outputHistoryLimit = 2 << 20 // 2 MiB of scrollback retained for reconnects

func newOutputHub() *outputHub {
	return &outputHub{subscribers: make(map[chan []byte]struct{})}
}

// subscribe registers a new viewer and returns a channel of future output
// plus a snapshot of everything already produced, so the caller can replay
// it before switching to live updates.
func (h *outputHub) subscribe() (ch chan []byte, replay []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch = make(chan []byte, 256)
	h.subscribers[ch] = struct{}{}
	replay = append([]byte(nil), h.history...)
	return ch, replay
}

func (h *outputHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
}

func (h *outputHub) publish(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.history = append(h.history, data...)
	if len(h.history) > outputHistoryLimit {
		h.history = h.history[len(h.history)-outputHistoryLimit:]
	}

	for ch := range h.subscribers {
		select {
		case ch <- data:
		default:
			// A slow/stuck viewer shouldn't be able to stall the shell;
			// it will simply catch up via history on its next reconnect.
		}
	}
}

// webSession represents one live run of runShellSession(), independent of
// how many (or how few) browser tabs are currently watching it.
type webSession struct {
	hub    *outputHub
	stdinW *os.File
	done   chan struct{} // closed once the shell exits (i.e. "exit" was typed)
}

var (
	sessionMu      sync.Mutex
	currentSession *webSession
)

// getOrCreateSession returns the currently running session, starting a new
// one (with a fresh banner) only if none is currently alive.
func getOrCreateSession() (*webSession, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if currentSession != nil {
		return currentSession, nil
	}

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		stdinR.Close()
		stdinW.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	origStdin, origStdout, origStderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin = stdinR
	os.Stdout = outW
	os.Stderr = outW

	s := &webSession{
		hub:    newOutputHub(),
		stdinW: stdinW,
		done:   make(chan struct{}),
	}
	currentSession = s

	// Runs the actual interactive shell.
	go func() {
		banner()
		fmt.Println("  Welcome, operator. (Web Console)")
		fmt.Println("  Type 'help' to access the command center.")
		fmt.Println("  Note: closing this tab or losing connection will NOT stop the session.")
		fmt.Println("        Only the 'exit' command shuts it down.")
		fmt.Println()
		runShellSession()

		// The shell only returns here once "exit" was typed (or stdin
		// itself was closed, which nothing in the web path ever does).
		os.Stdin, os.Stdout, os.Stderr = origStdin, origStdout, origStderr
		_ = outW.Close()
	}()

	// Continuously drains the shell's output into the hub, regardless of
	// whether any browser tab is currently attached to watch it.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := outR.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				s.hub.publish(chunk)
			}
			if readErr != nil {
				break
			}
		}

		close(s.done)

		sessionMu.Lock()
		if currentSession == s {
			currentSession = nil
		}
		sessionMu.Unlock()

		_ = stdinR.Close()
		_ = stdinW.Close()
	}()

	return s, nil
}

const webIndexHTML = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GO RUNCARNATION - WEB CONSOLE</title>
<style>
  * { box-sizing: border-box; }
  html, body {
    margin: 0; padding: 0; height: 100%;
    background: #0b0f0b; color: #39ff14;
    font-family: "Consolas", "SFMono-Regular", "Menlo", monospace;
  }
  #terminal {
    white-space: pre-wrap;
    word-break: break-word;
    padding: 16px;
    height: calc(100vh - 56px);
    overflow-y: auto;
    font-size: 13px;
    line-height: 1.45;
  }
  #inputBar {
    display: flex;
    align-items: center;
    border-top: 1px solid #1f4d1f;
    padding: 10px 16px;
    background: #050805;
    height: 56px;
  }
  #prompt { color: #39ff14; margin-right: 8px; opacity: 0.8; }
  #cmdInput {
    flex: 1;
    background: transparent;
    border: none;
    outline: none;
    color: #39ff14;
    font-family: inherit;
    font-size: 14px;
  }
  #status { position: fixed; top: 8px; right: 12px; font-size: 11px; opacity: 0.6; }
</style>
</head>
<body>
  <div id="status">connecting...</div>
  <div id="terminal"></div>
  <div id="inputBar">
    <span id="prompt">&gt;</span>
    <input id="cmdInput" type="text" autocomplete="off" spellcheck="false" autofocus />
  </div>
<script>
  const term = document.getElementById('terminal');
  const input = document.getElementById('cmdInput');
  const statusEl = document.getElementById('status');

  function append(text) {
    term.textContent += text;
    term.scrollTop = term.scrollHeight;
  }

  const es = new EventSource('/stream');

  es.onopen = () => { statusEl.textContent = 'connected'; };

  es.onmessage = (e) => {
    try {
      append(JSON.parse(e.data));
    } catch (err) {
      append(e.data);
    }
  };

  es.addEventListener('closed', () => {
    // The shell itself exited (someone typed "exit"). Stop the
    // EventSource so it doesn't auto-reconnect and silently start a
    // brand new session behind the operator's back.
    statusEl.textContent = 'session ended';
    append('\n[session ended - reload the page to start a new one]\n');
    input.disabled = true;
    es.close();
  });

  es.onerror = () => {
    // A dropped connection (tab backgrounded, network blip, etc.) does
    // NOT end the underlying shell session - it just keeps running.
    // EventSource retries automatically; once it reconnects it replays
    // anything that was missed.
    statusEl.textContent = 'reconnecting...';
  };

  input.addEventListener('keydown', async (ev) => {
    if (ev.key !== 'Enter') return;

    const line = input.value;
    input.value = '';

    try {
      await fetch('/input', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ line })
      });
    } catch (err) {
      append('\n[入力送信に失敗しました: ' + err + ']\n');
    }
  });

  input.focus();
</script>
</body>
</html>
`

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, webIndexHTML)
}

func writeSSEData(w io.Writer, payload string) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = []byte(strconv_Quote(payload))
	}
	fmt.Fprintf(w, "data: %s\n\n", encoded)
}

// strconv_Quote is a tiny fallback in case json.Marshal ever fails on a
// plain string (it shouldn't), avoiding an extra import purely for a
// near-impossible edge case.
func strconv_Quote(s string) string {
	return `"` + html.EscapeString(s) + `"`
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	s, err := getOrCreateSession()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ch, replay := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	if len(replay) > 0 {
		writeSSEData(w, string(replay))
		flusher.Flush()
	}

	for {
		select {

		case data, ok := <-ch:
			if !ok {
				return
			}
			writeSSEData(w, string(data))
			flusher.Flush()

		case <-s.done:
			// The shell itself exited (the "exit" command was typed).
			// This is the only case where we tell the browser to stop.
			fmt.Fprint(w, "event: closed\ndata: {}\n\n")
			flusher.Flush()
			return

		case <-r.Context().Done():
			// The browser disconnected. The session is left running
			// untouched; a future /stream request will re-attach to it.
			return
		}
	}
}

func handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Line string `json:"line"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Typing into the input box implicitly (re)attaches to - or starts -
	// the session, so input still works even if the SSE stream hasn't
	// finished reconnecting yet.
	s, err := getOrCreateSession()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := s.stdinW.Write([]byte(payload.Line + "\n")); err != nil {
		http.Error(w, "failed to deliver input", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// openBrowser launches the OS's default web browser pointed at url.
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux and other unix-likes
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}

// startWebServer starts the embedded web console and blocks forever.
func startWebServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/stream", handleStream)
	mux.HandleFunc("/input", handleInput)

	fmt.Println("[WEB] GO RUNCARNATION web console listening on", webListenAddr)
	fmt.Println("[WEB] Open " + webBrowserURL + " in your browser (opening automatically)...")

	if err := http.ListenAndServe(webListenAddr, mux); err != nil {
		fmt.Println("[ERROR] Web server failed:", err)
		os.Exit(1)
	}
}

func main() {

	// The web console plumbs its own os.Pipe()s for stdin/stdout. Writing to
	// one of these after its read end has been closed (which happens
	// routinely as sessions start and end) raises SIGPIPE; without this,
	// the default disposition would silently kill the whole process. We'd
	// rather just get an ordinary (and already-handled) write error.
	signal.Ignore(syscall.SIGPIPE)

	cleanupLogging, logErr := setupLogging()
	if logErr != nil {
		fmt.Println("[WARN] Logging could not be set up:", logErr)
		cleanupLogging = func() {}
	}

	if len(os.Args) > 1 {
		code := runNonInteractive(os.Args[1:])
		cleanupLogging()
		os.Exit(code)
	}

	defer cleanupLogging()

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(webBrowserURL)
	}()

	startWebServer()
}
