// Package bases содержит встроенные таблицы сечений, которые компилируются
// в бинарник csolve.
//
// Каждая таблица хранится рядом со своим текстовым исходником в сжатом виде
// (data.txt.gz) и встраивается через embed. Текстовые data.txt остаются
// источником правды: после их изменения нужно пересобрать .gz командой
//
//	go generate ./bases        (или make generate)
//
// Генератор детерминирован, поэтому повторная сборка не даёт диффов.
package bases

import "embed"

//go:generate go run ../internal/gen -dir .

// Встроены только наборы average: sphere (base_sph_25), spheroid
// (base_sphrd_25) и agglomerate (large_aggl_100).
//
//go:embed base_sph_25/average/*/*.gz
//go:embed base_sphrd_25/average/*/*.gz
//go:embed large_aggl_100/average/*/*.gz
var FS embed.FS
