// cli.go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hate/core"
)

const (
	buildFileName = "build.hate"

	// 映射表目录结构: mapping/{freak,lua}/
	// Mapping directory layout: mapping/{freak,lua}/
	mappingRootDir  = "mapping"
	freakMappingDir = "freak"
	luaMappingDir   = "lua"

	freakHookFile      = "hookfunction.json"
	freakMethodFile    = "method.json"
	freakObjectFile    = "object.json"
	freakReversedFile  = "order_reversed.json"
	freakWhitelistFile = "whitelist.json"
	freakOrdinaryFile  = "ordinary_function.json"
	translateFile      = "translate.json"
	luaCoreFile        = "lua.json"

	// ANSI 终端颜色 / ANSI terminal colors
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorReset  = "\033[0m"
)

// Options 是 compile 子命令的配置项，字段名对应命令行参数。
// Options holds compile subcommand settings; field names match CLI flags.
type Options struct {
	ConfigPath string // --config
	SrcDir     string // -cp
	OutDir     string // -out
	Select     string // -select
	All        bool   // -all
	AllSet     bool   // -all 是否显式传入 / whether -all was passed explicitly
	Modify     bool   // -modify
	ModifySet  bool   // -modify 是否显式传入 / whether -modify was passed explicitly
	ReqImp     string // -reqimp import|require，目标导入关键字，默认 require
	ReqImpSet  bool   // -reqimp 是否显式传入 / whether -reqimp was passed explicitly
}

// Run 是 CLI 入口；无参数时打印用法并等待回车，便于双击运行。
// Run is the CLI entry point; with no args it prints usage and waits for Enter.
func Run(args []string) {
	if len(args) == 0 {
		printUsage()
		fmt.Println("\n按回车键退出... / Press Enter to exit...")
		var input string
		fmt.Scanln(&input)
		return
	}

	switch cmd, subArgs := args[0], args[1:]; cmd {
	case "compile":
		runCompile(subArgs)
	case "init":
		runInit(subArgs)
	case "version", "-v", "--version":
		fmt.Println("hate 1.0.0")
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("%s[错误/ERROR] 未知命令/unknown command: %s%s\n", colorRed, cmd, colorReset)
		os.Exit(1)
	}
}

// runCompile 流程: 解析参数 → 定位工作区 → 合并配置 → 校验 → 转译。
// runCompile flow: parse args → locate workspace → merge config → validate → transpile.
func runCompile(args []string) {
	cliOptions, err := parseArgs(args)
	if err != nil {
		fmt.Printf("%s[错误/ERROR] %v%s\n", colorRed, err, colorReset)
		return
	}

	cwd, _ := os.Getwd()
	workspaceRoot, configPath := locateWorkspace(cwd, cliOptions.ConfigPath)
	if workspaceRoot == "" {
		workspaceRoot = cwd
		configPath = buildFileName
	} else {
		fmt.Printf("%s[系统/INFO] 自动定位到工作区/auto-located workspace: %s%s\n", colorGray, workspaceRoot, colorReset)
	}

	fileOptions := loadBuildFile(configPath)
	effective := merge(cliOptions, fileOptions)
	absolutePaths(effective, workspaceRoot)

	if err := validate(effective); err != nil {
		fmt.Printf("%s[错误/ERROR] %v%s\n", colorRed, err, colorReset)
		return
	}

	tables, err := loadMappingFiles(effective.Modify, workspaceRoot)
	if err != nil {
		fmt.Printf("%s[错误/ERROR] %v%s\n", colorRed, err, colorReset)
		return
	}
	mapping := core.LoadMapping(tables)
	if mapping == nil {
		fmt.Printf("%s[错误/ERROR] 映射表 JSON 解析失败/failed to parse mapping JSON%s\n", colorRed, colorReset)
		return
	}

	fmt.Printf("%s[任务/TASK] 开始转译/starting transpile...%s\n", colorCyan, colorReset)

	// Go 没有三元运算符，必须显式 if/else。
	// Go has no ternary operator; must use explicit if/else.
	var files []string
	if effective.All {
		files = core.WalkDir(effective.SrcDir)
	} else {
		files = []string{effective.Select}
	}

	reserveExistingNames(files, mapping)

	failures := 0
	for _, src := range files {
		fmt.Printf("  -> 处理/processing: %s\n", filepath.Base(src))
		if !core.CompileFile(src, effective.SrcDir, effective.OutDir, mapping, effective.ReqImp) {
			failures++
		}
	}

	if failures > 0 {
		fmt.Printf("%s[失败/FAIL] %d 个文件转译失败/%d file(s) failed%s\n", colorRed, failures, failures, colorReset)
	} else {
		fmt.Printf("%s[完成/DONE] 成功编译/successfully compiled %d 个文件/file(s)!%s\n", colorGreen, len(files), colorReset)
	}
}

// locateWorkspace 显式给了 config 用 cwd；否则从 cwd 向上查找 build.hate。
// locateWorkspace uses cwd when config is explicit; otherwise searches upward from cwd.
func locateWorkspace(cwd, explicitConfig string) (workspaceRoot, configPath string) {
	if explicitConfig != "" {
		return cwd, explicitConfig
	}
	return findWorkspace(cwd)
}

// findWorkspace 从 startDir 向上查找 build.hate，找到返回 (所在目录, 文件路径)，否则返回空串。
// findWorkspace searches upward from startDir for build.hate.
func findWorkspace(startDir string) (string, string) {
	dir := startDir
	for {
		cfgPath := filepath.Join(dir, buildFileName)
		if info, err := os.Stat(cfgPath); err == nil && !info.IsDir() {
			return dir, cfgPath
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// absolutePaths 把相对路径按 workspaceRoot 解析为绝对路径。
// absolutePaths resolves relative paths against workspaceRoot.
func absolutePaths(o *Options, workspaceRoot string) {
	toAbs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(workspaceRoot, p)
	}
	o.SrcDir = toAbs(o.SrcDir)
	o.OutDir = toAbs(o.OutDir)
	o.Select = toAbs(o.Select)
}

// loadMappingFiles 读取全部映射表原始字节。
// loadMappingFiles reads all mapping files and returns raw bytes.
func loadMappingFiles(modifyMode bool, workspaceRoot string) (*core.MappingTables, error) {
	var root string
	if modifyMode {
		root = filepath.Join(workspaceRoot, mappingRootDir)
		fmt.Printf("%s[提示/HINT] 使用工作区映射表/using workspace mappings: %s%s\n", colorYellow, root, colorReset)
	} else {
		exePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("无法定位 exe 路径/cannot locate exe path: %w", err)
		}
		root = filepath.Join(filepath.Dir(exePath), mappingRootDir)
	}

	freakDir := filepath.Join(root, freakMappingDir)
	luaDir := filepath.Join(root, luaMappingDir)

	tables := &core.MappingTables{}
	var err error

	if tables.FreakHook, err = readFile(freakDir, freakHookFile); err != nil {
		return nil, err
	}
	if tables.FreakMethod, err = readFile(freakDir, freakMethodFile); err != nil {
		return nil, err
	}
	if tables.FreakObject, err = readFile(freakDir, freakObjectFile); err != nil {
		return nil, err
	}
	if tables.FreakReversed, err = readFile(freakDir, freakReversedFile); err != nil {
		return nil, err
	}
	if tables.FreakWhitelist, err = readFile(freakDir, freakWhitelistFile); err != nil {
		return nil, err
	}
	if tables.FreakOrdinary, err = readFile(freakDir, freakOrdinaryFile); err != nil {
		return nil, err
	}
	if tables.FreakSemantics, err = readFile(freakDir, translateFile); err != nil {
		return nil, err
	}
	if tables.LuaCore, err = readFile(luaDir, luaCoreFile); err != nil {
		return nil, err
	}

	return tables, nil
}

// readFile 读取 dir 下的映射文件。
// readFile reads a mapping file under dir.
func readFile(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("找不到映射表/mapping file not found: %s", path)
	}
	return data, nil
}

// reserveExistingNames 扫描源码里已有的 ASCII 标识符并预占，避免自动生成的名字与之冲突。
// reserveExistingNames scans source files for existing ASCII identifiers and pre-claims them to avoid collisions.
func reserveExistingNames(files []string, mapping *core.Mapping) {
	for _, src := range files {
		content, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		for _, tok := range core.Tokenize(string(content)) {
			if tok.IsIdentifier() && core.IsASCII(tok.Text) {
				mapping.Names.Reserve(tok.Text)
			}
		}
	}
}

func runInit(args []string) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		fmt.Printf("%s[错误/ERROR] 无法创建目录/cannot create directory %s: %v%s\n", colorRed, dir, err, colorReset)
		return
	}

	cfgPath := filepath.Join(dir, buildFileName)
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("%s[警告/WARN] %s 已存在，跳过/already exists, skipping%s\n", colorYellow, cfgPath, colorReset)
		return
	}

	// 全部使用相对路径，便于项目整体搬迁。
	// Use relative paths throughout for easy project relocation.
	content := strings.Join([]string{
		"# hate 编译配置 (与 haxe/lua 习惯一致: 键值以空格分隔) / hate build config (key-value separated by space)",
		"-cp ./scripts",
		"-all true",
		"-out ./out",
		"-reqimp require",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		fmt.Printf("%s[错误/ERROR] 写入失败/failed to write %s: %v%s\n", colorRed, cfgPath, err, colorReset)
		return
	}
	fmt.Printf("%s[成功/OK] 已生成/generated %s%s\n", colorGreen, cfgPath, colorReset)
}

// parseArgs 解析 compile 子命令的命令行参数。
// parseArgs parses command-line arguments for the compile subcommand.
func parseArgs(args []string) (*Options, error) {
	options := &Options{}
	for i := 0; i < len(args); i++ {
		switch key := args[i]; key {
		case "--config":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--config 缺少值/missing value")
			}
			options.ConfigPath = args[i]
		case "-cp":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-cp 缺少值/missing value")
			}
			options.SrcDir = args[i]
		case "-all":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-all 缺少值/missing value")
			}
			options.AllSet = true
			options.All = args[i] == "true"
		case "-out":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-out 缺少值/missing value")
			}
			options.OutDir = args[i]
		case "-select":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-select 缺少值/missing value")
			}
			options.Select = args[i]
		case "-modify":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-modify 缺少值/missing value")
			}
			options.ModifySet = true
			options.Modify = args[i] == "true"
		case "-reqimp":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-reqimp 缺少值/missing value")
			}
			v := args[i]
			if v != "import" && v != "require" {
				return nil, fmt.Errorf("-reqimp 只能是 import 或 require/-reqimp must be import or require: %s", v)
			}
			options.ReqImpSet = true
			options.ReqImp = v
		default:
			return nil, fmt.Errorf("未知参数/unknown argument: %s", key)
		}
	}
	return options, nil
}

// 解析 build.hate，忽略空行与 # 注释。
// Parses build.hate, ignoring blank lines and # comments.
func loadBuildFile(path string) *Options {
	options := &Options{}
	data, err := os.ReadFile(path)
	if err != nil {
		return options
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		key := parts[0]
		value := ""
		if len(parts) > 1 {
			value = strings.TrimSpace(parts[1])
		}
		switch key {
		case "-cp":
			options.SrcDir = value
		case "-all":
			options.AllSet = true
			options.All = value == "true"
		case "-out":
			options.OutDir = value
		case "-select":
			options.Select = value
		case "-modify":
			options.ModifySet = true
			options.Modify = value == "true"
		case "-reqimp":
			if value == "import" || value == "require" {
				options.ReqImpSet = true
				options.ReqImp = value
			}
		}
	}
	return options
}

// merge 合并命令行与配置文件，命令行优先。
// 注意: 只有命令行显式传了 (Set 标志为 true) 才覆盖文件值，
// 否则保留 build.hate 中的设置。
// merge combines command-line and config-file options; command line takes precedence.
// Only flags explicitly passed on the command line (Set flag true) override file values.
func merge(cliOptions, fileOptions *Options) *Options {
	effective := &Options{}

	if cliOptions.SrcDir != "" {
		effective.SrcDir = cliOptions.SrcDir
	} else {
		effective.SrcDir = fileOptions.SrcDir
	}

	if cliOptions.AllSet {
		effective.All = cliOptions.All
	} else {
		effective.All = fileOptions.All
	}
	effective.AllSet = cliOptions.AllSet || fileOptions.AllSet

	if cliOptions.OutDir != "" {
		effective.OutDir = cliOptions.OutDir
	} else {
		effective.OutDir = fileOptions.OutDir
	}

	if cliOptions.Select != "" {
		effective.Select = cliOptions.Select
	} else {
		effective.Select = fileOptions.Select
	}

	if cliOptions.ModifySet {
		effective.Modify = cliOptions.Modify
	} else {
		effective.Modify = fileOptions.Modify
	}
	effective.ModifySet = cliOptions.ModifySet || fileOptions.ModifySet

	if cliOptions.ReqImpSet {
		effective.ReqImp = cliOptions.ReqImp
	} else {
		effective.ReqImp = fileOptions.ReqImp
	}
	effective.ReqImpSet = cliOptions.ReqImpSet || fileOptions.ReqImpSet

	// 默认值: require
	// Default: require
	if effective.ReqImp == "" {
		effective.ReqImp = "require"
	}

	return effective
}

func validate(o *Options) error {
	if o.SrcDir == "" {
		return fmt.Errorf("缺少 -cp (源目录)/missing -cp (source directory)")
	}
	if o.OutDir == "" {
		return fmt.Errorf("缺少 -out (输出目录)/missing -out (output directory)")
	}
	if info, err := os.Stat(o.SrcDir); err != nil || !info.IsDir() {
		return fmt.Errorf("-cp 不存在或不是目录/does not exist or is not a directory: %s", o.SrcDir)
	}
	if !o.All && o.Select == "" {
		return fmt.Errorf("-all false 时必须给 -select/-select is required when -all is false")
	}
	if o.ReqImp != "" && o.ReqImp != "import" && o.ReqImp != "require" {
		return fmt.Errorf("-reqimp 只能是 import 或 require/-reqimp must be import or require: %s", o.ReqImp)
	}
	return nil
}

func printUsage() {
	fmt.Printf("%shate Compiler v1.0.0%s\n", colorCyan, colorReset)
	fmt.Println("用法 / Usage:")
	fmt.Println("  hate compile [options]   编译 Lua (自动寻找 build.hate) / compile Lua (auto-finds build.hate)")
	fmt.Println("  hate init [dir]          生成初始 build.hate / generate initial build.hate")
	fmt.Println("  hate version             显示版本 / show version")
	fmt.Println("  hate help                显示本帮助 / show this help")
	fmt.Println("\ncompile 选项 / options:")
	fmt.Println("  --config <file>         指定配置文件路径 (默认向上查找 build.hate) / config file path (default: search upward for build.hate)")
	fmt.Println("  -modify true|false      是否使用工作区的自定义 JSON 映射 (默认 false) / use workspace custom JSON mappings (default false)")
	fmt.Println("  -reqimp import|require  目标导入关键字 (默认 require) / target import keyword (default require)")
	fmt.Println("  -cp <dir>               源目录 / source directory")
	fmt.Println("  -all true|false         编译全部 / compile everything")
	fmt.Println("  -out <dir>              输出目录 / output directory")
	fmt.Println("  -select <file>          -all false 时指定单文件 / single file when -all is false")
}