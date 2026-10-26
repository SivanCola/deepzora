//go:build !windows

package desktopinstance

func PrepareInstall(root, home string, interactive bool) (func(), error) { return func() {}, nil }
func CheckInstallVacant(root, home string) error                         { return nil }
func AttemptLog(home, action, root string) func(error) {
	return func(error) {}
}
