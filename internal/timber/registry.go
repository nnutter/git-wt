package timber

type registeredRepo struct {
	Name     string
	BarePath string
}

func (x registeredRepo) originURL(runtime Runtime) string {
	result, err := gitOutput(runtime, x.BarePath, "remote", "get-url", remoteName)
	if err != nil {
		return ""
	}
	return result.stdout
}
