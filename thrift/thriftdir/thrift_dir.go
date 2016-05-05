package thriftdir

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"path/filepath"

	"github.com/thriftrw/thriftrw-go/compile"
	"github.com/yarpc/yab/thrift"
)

type Match struct {
	File    string
	Service string
	Method  string
	cost    int
}

type matcher struct {
	routingSvc              string
	thriftSvc, thriftMethod string
	bestMatch               Match
	curFile                 string
}

func (m *matcher) loadFiles(files []string) {
	for _, file := range files {
		module, err := thrift.Parse(file)
		if err != nil {
			continue
		}

		m.curFile = file
		m.walkModule(module)
	}
}

func (m *matcher) walkModule(module *compile.Module) {
	for _, svc := range module.Services {
		for _, funcName := range svc.Functions {
			m.checkMethod(svc.Name, funcName.Name)
		}
	}
}

func (m *matcher) checkMethod(svc string, method string) {
	cost := editDistance(m.thriftSvc, svc) + editDistance(m.thriftMethod, method)
	if cost < 5 {
		fmt.Println("edit", m.thriftSvc, svc, editDistance(m.thriftSvc, svc))
		fmt.Println("cost", cost, svc, method)
	}

	if cost < m.bestMatch.cost {
		m.bestMatch = Match{
			File:    m.curFile,
			Service: svc,
			Method:  method,
			cost:    cost,
		}
	}
}

// FindMatching attempts to find a file that matches the passed in method
// based on the edit distance.
func FindMatching(dir, service, fullMethod string) Match {
	bestMatch := Match{cost: math.MaxInt64}

	thriftSvc, thriftMethod, _ := thrift.SplitMethod(fullMethod)
	if thriftSvc == "" {
		return bestMatch
	}

	matcher := matcher{
		routingSvc:   strings.ToLower(service),
		thriftSvc:    strings.ToLower(thriftSvc),
		thriftMethod: strings.ToLower(thriftMethod),
		bestMatch:    bestMatch,
	}

	files := getThriftFiles(dir)
	sort.Sort(sort.Reverse(byFileScore{
		parts: []string{matcher.routingSvc, matcher.thriftSvc},
		files: files,
	}))
	fmt.Println("sorted", files[:10])

	matcher.loadFiles(files)
	return matcher.bestMatch
}

func getThriftFiles(dir string) []string {
	var files []string
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() || !strings.HasSuffix(path, ".thrift") {
			return nil
		}

		files = append(files, path)
		return nil
	})
	return files
}

type byFileScore struct {
	parts []string
	files []string
}

func (p byFileScore) Len() int      { return len(p.files) }
func (p byFileScore) Swap(i, j int) { p.files[i], p.files[j] = p.files[j], p.files[i] }

func (p byFileScore) Less(i, j int) bool {
	return p.cost(p.files[i]) < p.cost(p.files[j])
}

func (p byFileScore) cost(path string) int {
	score := 0
	pathLower := strings.ToLower(path)
	for _, p := range p.parts {
		if strings.Contains(pathLower, p) {
			score++
		}
	}
	if score > 0 {
		fmt.Println("score", score, path)
	}
	return score
}

func calcScore(want string, got string) int {
	switch {
	case strings.EqualFold(want, got):
		return 0
	default:
		return editDistance(want, got)
	}
}
