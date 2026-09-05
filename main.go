package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// pollInterval is how often runcarnation checks GitHub for new commits
// while the application is running.
const pollInterval = 5 * time.Second

// logRetention is how long a log file is kept before cleanOldLogs removes it.
const logRetention = 14 * 24 * time.Hour

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

func main() {

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

	banner()

	fmt.Println("  Welcome, operator.")
	fmt.Println("  Type 'help' to access the command center.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

MainLoop:
	for {

		fmt.Print("GO RUNCARNATION> ")

		if !scanner.Scan() {
			break MainLoop
		}

		input := normalizeCommand(strings.TrimSpace(scanner.Text()))

		switch input {

		case "help":
			help()

		case "check", "status", "remote", "fetch", "diff", "ignored", "changed", "run", "runcarnation":
			path, eof := readRepoPath(scanner)

			if eof {
				break MainLoop
			}

			if path == "" {
				fmt.Println("[ERROR]", t("err_path_empty"))
				continue MainLoop
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
