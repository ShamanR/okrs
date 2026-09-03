package httproute_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// rawPathInLog ловит запись запрошенного пути под ключом path: `"path", r.URL.Path`
// с любым именем переменной запроса.
var rawPathInLog = regexp.MustCompile(`"path",\s*\w+\.URL\.Path`)

// Правило «в лог идёт шаблон маршрута, а не путь» обязано действовать для всех
// записей сразу, а не для той, которую вспомнили.
//
// Гард сторожевой, а не декоративный: правило уже существовало тремя копиями,
// и закрытой оказалась одна — запись о запросе. Токен приглашения продолжал
// утекать через запись о панике и запись об отказе в доступе, и каждую пришлось
// чинить отдельно, после отдельного code review. Обычный тест такого не ловит:
// он проверяет запись, которая есть, а не ту, которую кто-то добавит завтра.
//
// Единственное законное место, где путь превращается в значение поля, —
// httproute.Pattern.
func TestNoRecordWritesTheRawRequestPath(t *testing.T) {
	root := filepath.Join("..", "..")
	own, err := filepath.Abs(filepath.Join("..", "httproute"))
	if err != nil {
		t.Fatalf("не удалось определить собственный каталог: %v", err)
	}

	var offenders []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "openspec", "docs", "web":
				return fs.SkipDir
			}
			abs, absErr := filepath.Abs(path)
			if absErr == nil && abs == own {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(src), "\n") {
			if rawPathInLog.MatchString(line) {
				offenders = append(offenders, filepath.ToSlash(path)+":"+strconv.Itoa(i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход дерева не удался: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("запрошенный путь пишется в лог напрямую в %v;\n"+
			"значение параметра маршрута может быть учётными данными — используйте httproute.Pattern", offenders)
	}
}
