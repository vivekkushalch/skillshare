package version

// CheckNow bypasses the automatic-check policy and cache for an explicit request.
func CheckNow(current string, method InstallMethod) (*CheckResult, error) {
	latest, err := latestVersionFor(method)
	if err != nil {
		return nil, err
	}
	available, err := compareVersions(current, latest)
	if err != nil {
		return nil, err
	}
	return &CheckResult{CurrentVersion: current, LatestVersion: latest, UpdateAvailable: available, InstallMethod: method}, nil
}

// latestVersionFor asks the source the install updates from: Homebrew, PyPI,
// or GitHub releases for everything else.
func latestVersionFor(method InstallMethod) (string, error) {
	switch {
	case method == InstallBrew:
		return FetchBrewLatestVersion()
	case method.IsPyPI():
		return FetchPyPILatestVersion()
	}
	return FetchLatestVersionOnly()
}
