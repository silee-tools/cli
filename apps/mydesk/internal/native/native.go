package native

type NativeMode int

const (
	ModeLink   NativeMode = iota // home/, config/, ssh/ — file-level symlinks
	ModeVSCode                   // vscode/ — symlink + extensions management
	ModeBrew                     // brew/ — Brewfile sync/install
	ModeScript                   // macos/, omz/ — script execution
)

type NativeDir struct {
	Dir        string     // directory name in config repo (e.g., "home")
	TargetBase string     // system target path (e.g., "~")
	Mode       NativeMode
}

func Dirs() []NativeDir {
	return []NativeDir{
		{Dir: "home", TargetBase: "~", Mode: ModeLink},
		{Dir: "config", TargetBase: "~/.config", Mode: ModeLink},
		{Dir: "ssh", TargetBase: "~/.ssh", Mode: ModeLink},
		{Dir: "vscode", TargetBase: "~/Library/Application Support/Code/User", Mode: ModeVSCode},
		{Dir: "brew", TargetBase: "", Mode: ModeBrew},
		{Dir: "macos", TargetBase: "", Mode: ModeScript},
		{Dir: "omz", TargetBase: "", Mode: ModeScript},
	}
}

func LinkDirs() []NativeDir {
	var result []NativeDir
	for _, d := range Dirs() {
		if d.Mode == ModeLink || d.Mode == ModeVSCode {
			result = append(result, d)
		}
	}
	return result
}
