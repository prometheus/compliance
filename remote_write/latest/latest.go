package latest

import (
        "encoding/json"
        "io/ioutil"
        "log"
        "net/http"
	"regexp"
        "strings"
        "time"
)

func GetLatestVersion(repo string) string {
        url := "https://api.github.com/repos/" + repo + "/releases/latest"

        httpClient := http.Client{
                Timeout: time.Second * 2, // Timeout after 2 seconds
        }

        req, err := http.NewRequest(http.MethodGet, url, nil)
        if err != nil {
                log.Fatal(err)
        }

        req.Header.Add("Accept", "application/vnd.github.v3+json")

        res, getErr := httpClient.Do(req)
        if getErr != nil {
                log.Fatal(getErr)
        }

        if res.Body != nil {
                defer res.Body.Close()
        }

        body, readErr := ioutil.ReadAll(res.Body)
        if readErr != nil {
                log.Fatal(readErr)
        }

        type PackageInfo struct {
                TagName         string    `json:"tag_name"`
                CreatedAt       time.Time `json:"created_at"`
                PublishedAt     time.Time `json:"published_at"`
        }

        var packageInfo PackageInfo

        jsonErr := json.Unmarshal(body, &packageInfo)
        if jsonErr != nil {
                log.Fatal(jsonErr)
        }

        version := strings.Trim(packageInfo.TagName, "v")
//        fmt.Println("repository: " + repo + " - version: " + version)

        return version
}

func GetDownloadURL(url string) string {
	re := regexp.MustCompile("https://github.com/(.+?/.+?)/.*")
	repo := re.ReplaceAllString(url, "$1")
        version := GetLatestVersion(repo)

	re2 := regexp.MustCompile("(.*)VERSION(.*)")
	url = re2.ReplaceAllString(url, "${1}" + version + "${2}")

        return url
}
