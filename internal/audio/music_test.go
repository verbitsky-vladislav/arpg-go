package audio

import (
	"os"
	"testing"
	"testing/fstest"
)

// Проигрывание проверить без устройства нельзя, а вот разбор манифеста и
// логику перехода — можно, и именно там живут ошибки, которые слышны как
// «музыка пропала» или «два трека играют разом».

func TestMusicLoadsForestTrack(t *testing.T) {
	m, err := loadMusic(os.DirFS("../../assets"), "audio/music")
	if err != nil {
		t.Fatalf("музыка не загрузилась: %v", err)
	}
	tr, ok := m.tracks["forest"]
	if !ok {
		t.Fatal("в манифесте нет трека forest")
	}
	if tr.gain <= 0 || tr.gain > 1 {
		t.Errorf("громкость трека %.2f вне (0, 1]", tr.gain)
	}
	// Поток приводится к стерео 16 бит на частоте контекста: 4 байта на кадр.
	if tr.size%4 != 0 {
		t.Errorf("%d байт — не целое число стерео-кадров", tr.size)
	}
	// Луп на минуту с лишним: короче — слышно, что это петля.
	if sec := float64(tr.size) / (SampleRate * 4); sec < 30 {
		t.Errorf("трек длиной %.1f с — для фонового лупа слишком коротко", sec)
	}
}

func TestMissingMusicIsNotAnError(t *testing.T) {
	// Биомов много, треков пока один: забег в биоме без музыки должен идти в
	// тишине, а не падать.
	m, err := loadMusic(fstest.MapFS{}, "audio/music")
	if err != nil {
		t.Fatalf("отсутствие манифеста музыки стало ошибкой: %v", err)
	}
	if len(m.tracks) != 0 {
		t.Errorf("треков без манифеста: %d", len(m.tracks))
	}
	m.play("forest") // не должно паниковать
}

func TestMusicIgnoresRepeatedRequest(t *testing.T) {
	// Сцена зовёт Music каждый кадр — повторный вызов обязан быть пустым,
	// иначе трек перезапускался бы 60 раз в секунду.
	m := &music{tracks: map[string]musicTrack{"forest": {raw: make([]byte, 64), size: 64, gain: 1}}}
	m.play("forest")
	if m.cur != "forest" {
		t.Fatalf("трек не выбран: %q", m.cur)
	}
	m.fade = 7 // как будто переход в разгаре
	m.play("forest")
	if m.fade != 7 {
		t.Error("повторный вызов перезапустил переход")
	}
}

func TestMusicSwitchStartsCrossfade(t *testing.T) {
	m := &music{tracks: map[string]musicTrack{
		"forest": {raw: make([]byte, 64), size: 64, gain: 1},
		"cave":   {raw: make([]byte, 64), size: 64, gain: 1},
	}}
	m.play("forest")
	m.play("cave")
	if m.cur != "cave" {
		t.Errorf("текущий трек %q, ожидался cave", m.cur)
	}
	if m.fade != fadeTicks {
		t.Errorf("переход длиной %d тиков, ожидалось %d", m.fade, fadeTicks)
	}
}

func TestMusicStopsOnEmptyID(t *testing.T) {
	// Пустой id — уход в тишину (так главное меню гасит музыку забега).
	m := &music{tracks: map[string]musicTrack{"forest": {raw: make([]byte, 64), size: 64, gain: 1}}}
	m.play("forest")
	m.play("")
	if m.cur != "" {
		t.Errorf("текущий трек %q, ожидалась тишина", m.cur)
	}
	if m.now != nil {
		t.Error("после ухода в тишину остался играющий плеер")
	}
}

func TestMusicUpdateRunsDownTheFade(t *testing.T) {
	m := &music{tracks: map[string]musicTrack{}}
	m.play("forest") // трека нет, но счётчик перехода заводится
	for i := 0; i < fadeTicks+5; i++ {
		m.update(1)
	}
	if m.fade != 0 {
		t.Errorf("переход не завершился: осталось %d тиков", m.fade)
	}
}

func TestNilMusicIsSafe(t *testing.T) {
	// Банк, собранный в тестах вручную, музыки не имеет — и не должен падать.
	var m *music
	m.play("forest")
	m.update(1)
}
