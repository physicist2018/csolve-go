// Command gen пересобирает gzip-копии встроенных таблиц сечений.
//
// Таблицы лежат в bases/<набор>/average/mre_*/data.txt; рядом с каждым
// исходником пишется data.txt.gz, который и встраивается в бинарник
// (см. bases/embed.go). Запуск: go generate ./bases.
//
// Вывод детерминирован: gzip-заголовок без имени файла и времени, поэтому
// повторная генерация не меняет байты и не создаёт диффов в git.
package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// builtinSets — каталоги встроенных наборов таблиц относительно bases/.
var builtinSets = []string{
	"base_sph_25/average",
	"base_sphrd_25/average",
	"large_aggl_100/average",
}

func main() {
	dir := flag.String("dir", ".", "каталог с наборами таблиц (bases)")
	force := flag.Bool("force", false, "пересобрать, даже если .gz новее исходника")
	flag.Parse()

	written, skipped := 0, 0
	for _, set := range builtinSets {
		root := filepath.Join(*dir, set)
		entries, err := os.ReadDir(root)
		if err != nil {
			die("%v", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			src := filepath.Join(root, e.Name(), "data.txt")
			dst := src + ".gz"
			if !*force && upToDate(src, dst) {
				skipped++
				continue
			}
			if err := compress(src, dst); err != nil {
				die("%v", err)
			}
			written++
		}
	}
	fmt.Printf("gen: %d .gz записано, %d без изменений\n", written, skipped)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", a...)
	os.Exit(1)
}

// upToDate — .gz существует и не старше исходного data.txt.
func upToDate(src, dst string) bool {
	si, err := os.Stat(src)
	if err != nil {
		die("%v", err)
	}
	di, err := os.Stat(dst)
	if err != nil || di.Size() == 0 {
		return false
	}
	return !si.ModTime().After(di.ModTime())
}

func compress(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		f.Close()
		return err
	}
	// Header по умолчанию: без имени, mtime = 0, OS = 255 — детерминированно.
	if _, err := zw.Write(data); err != nil {
		zw.Close()
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
