# Герои — визуальная витрина

Статус: 🟢 спрайты готовы (6 героев × 3 вида × 6 анимаций)
Связь: `../mechanics/characters.md` (роли/лор/умения), `art-direction.md` (как рисуем),
`to-draw.md` (бэклог), код — `internal/hero`, `internal/anim`

Наглядный каталог **всех героев в движении**: каждая анимация — живой GIF на
тёмном игровом фоне, скорость = как в игре. Здесь «как выглядит»; роли, защита и
билды — в `../mechanics/characters.md`.

**Направления (top-down).** Три реальных вида: **front** (`down`), **back**
(`up`) и **side** (профиль лицом вправо). Взгляд **влево** — это side,
**отражённый** по горизонтали (в игре при движении влево). Итого 4 направления
взгляда из 3 наборов кадров: front / back / вправо / влево.

> Картинки — чистым Markdown (`![](...)`), анимация видна в любом просмотрщике.
> Кадры-исходники: `assets/sprites/heroes/<id>/<down|up|side>/<action>/` (16 PNG).
> Перегенерация: `go run ./tools/spritegen assets/sprites/heroes` → `bash tools/gifgen.sh`.

## Анимации (что показываем)

| Действие | Что это |
|----------|---------|
| **idle** | покой — лёгкое «дыхание» |
| **walk** | ходьба — попеременный шаг |
| **attack** | удар оружием (замах стартового умения) |
| **punch** | базовый удар (без оружия) |
| **take_damage** | получение урона — красная вспышка + отдача |
| **loot** | подбор предмета — наклон к земле |

Каждая анимация — **16 кадров**, нативно 32×32, апскейл ×8 (в игре 256×256; в
витрине 128px). `down` — исходный `base.png`; `up` (спина) и `side` (профиль)
строятся в `tools/spritegen` (спина — заливкой лица, профиль — параметрической
пиксель-отрисовкой в палитре героя). Подробности — `art-direction.md`.

---

## 💪 Сила → Броня

### ⚔️ Рыцарь (knight) — стойкость
**Сила · Броня · ближний физ.** · Старт-умение: **Раскол** · Ресурс: Мана
«Я — стена и меч. Держу строй, принимаю удар, бью в ответ.»

| вид | idle | walk | attack | punch | take_damage | loot |
|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| front | ![](media/heroes/knight/down/idle.gif) | ![](media/heroes/knight/down/walk.gif) | ![](media/heroes/knight/down/attack.gif) | ![](media/heroes/knight/down/punch.gif) | ![](media/heroes/knight/down/take_damage.gif) | ![](media/heroes/knight/down/loot.gif) |
| back | ![](media/heroes/knight/up/idle.gif) | ![](media/heroes/knight/up/walk.gif) | ![](media/heroes/knight/up/attack.gif) | ![](media/heroes/knight/up/punch.gif) | ![](media/heroes/knight/up/take_damage.gif) | ![](media/heroes/knight/up/loot.gif) |
| side | ![](media/heroes/knight/side/idle.gif) | ![](media/heroes/knight/side/walk.gif) | ![](media/heroes/knight/side/attack.gif) | ![](media/heroes/knight/side/punch.gif) | ![](media/heroes/knight/side/take_damage.gif) | ![](media/heroes/knight/side/loot.gif) |

### 🪓 Дикарь (barbarian) — ярость
**Сила · Броня · ближний физ. + вампиризм** · Старт-умение: **Вихрь** · Ресурс: Здоровье
«Ярость во плоти. Чем жарче бой, тем я сильнее.»

| вид | idle | walk | attack | punch | take_damage | loot |
|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| front | ![](media/heroes/barbarian/down/idle.gif) | ![](media/heroes/barbarian/down/walk.gif) | ![](media/heroes/barbarian/down/attack.gif) | ![](media/heroes/barbarian/down/punch.gif) | ![](media/heroes/barbarian/down/take_damage.gif) | ![](media/heroes/barbarian/down/loot.gif) |
| back | ![](media/heroes/barbarian/up/idle.gif) | ![](media/heroes/barbarian/up/walk.gif) | ![](media/heroes/barbarian/up/attack.gif) | ![](media/heroes/barbarian/up/punch.gif) | ![](media/heroes/barbarian/up/take_damage.gif) | ![](media/heroes/barbarian/up/loot.gif) |
| side | ![](media/heroes/barbarian/side/idle.gif) | ![](media/heroes/barbarian/side/walk.gif) | ![](media/heroes/barbarian/side/attack.gif) | ![](media/heroes/barbarian/side/punch.gif) | ![](media/heroes/barbarian/side/take_damage.gif) | ![](media/heroes/barbarian/side/loot.gif) |

---

## 🏹 Ловкость → Уклонение

### 🎯 Следопыт (ranger) — кайт
**Ловкость · Уклонение · дальний физ. + крит** · Старт-умение: **Пронзающая стрела** · Ресурс: Мана
«Точность и дистанция. Бью раньше, чем они дойдут.»

| вид | idle | walk | attack | punch | take_damage | loot |
|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| front | ![](media/heroes/ranger/down/idle.gif) | ![](media/heroes/ranger/down/walk.gif) | ![](media/heroes/ranger/down/attack.gif) | ![](media/heroes/ranger/down/punch.gif) | ![](media/heroes/ranger/down/take_damage.gif) | ![](media/heroes/ranger/down/loot.gif) |
| back | ![](media/heroes/ranger/up/idle.gif) | ![](media/heroes/ranger/up/walk.gif) | ![](media/heroes/ranger/up/attack.gif) | ![](media/heroes/ranger/up/punch.gif) | ![](media/heroes/ranger/up/take_damage.gif) | ![](media/heroes/ranger/up/loot.gif) |
| side | ![](media/heroes/ranger/side/idle.gif) | ![](media/heroes/ranger/side/walk.gif) | ![](media/heroes/ranger/side/attack.gif) | ![](media/heroes/ranger/side/punch.gif) | ![](media/heroes/ranger/side/take_damage.gif) | ![](media/heroes/ranger/side/loot.gif) |

### 🗡️ Ассасин (assassin) — бурст
**Ловкость · Уклонение · ближний физ. + крит** · Старт-умение: **Теневой рывок** · Ресурс: Мана
«Тень и клинок. Врываюсь, взрываю цель, исчезаю.»

| вид | idle | walk | attack | punch | take_damage | loot |
|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| front | ![](media/heroes/assassin/down/idle.gif) | ![](media/heroes/assassin/down/walk.gif) | ![](media/heroes/assassin/down/attack.gif) | ![](media/heroes/assassin/down/punch.gif) | ![](media/heroes/assassin/down/take_damage.gif) | ![](media/heroes/assassin/down/loot.gif) |
| back | ![](media/heroes/assassin/up/idle.gif) | ![](media/heroes/assassin/up/walk.gif) | ![](media/heroes/assassin/up/attack.gif) | ![](media/heroes/assassin/up/punch.gif) | ![](media/heroes/assassin/up/take_damage.gif) | ![](media/heroes/assassin/up/loot.gif) |
| side | ![](media/heroes/assassin/side/idle.gif) | ![](media/heroes/assassin/side/walk.gif) | ![](media/heroes/assassin/side/attack.gif) | ![](media/heroes/assassin/side/punch.gif) | ![](media/heroes/assassin/side/take_damage.gif) | ![](media/heroes/assassin/side/loot.gif) |

---

## 🔮 Интеллект → Энергощит

### 🔥 Маг (mage) — стихии
**Интеллект · Энергощит · дальние чары** · Старт-умение: **Огненный шар** · Ресурс: Мана
«Буря стихий. Выжигаю толпы издалека.»

| вид | idle | walk | attack | punch | take_damage | loot |
|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| front | ![](media/heroes/mage/down/idle.gif) | ![](media/heroes/mage/down/walk.gif) | ![](media/heroes/mage/down/attack.gif) | ![](media/heroes/mage/down/punch.gif) | ![](media/heroes/mage/down/take_damage.gif) | ![](media/heroes/mage/down/loot.gif) |
| back | ![](media/heroes/mage/up/idle.gif) | ![](media/heroes/mage/up/walk.gif) | ![](media/heroes/mage/up/attack.gif) | ![](media/heroes/mage/up/punch.gif) | ![](media/heroes/mage/up/take_damage.gif) | ![](media/heroes/mage/up/loot.gif) |
| side | ![](media/heroes/mage/side/idle.gif) | ![](media/heroes/mage/side/walk.gif) | ![](media/heroes/mage/side/attack.gif) | ![](media/heroes/mage/side/punch.gif) | ![](media/heroes/mage/side/take_damage.gif) | ![](media/heroes/mage/side/loot.gif) |

### ⚙️ Инженер (engineer) — зона/турели
**Интеллект · Энергощит · дальний/непрямой** · Старт-умение: **Турель** · Ресурс: Мана
«Бьют не руки, а мои машины. Держу зону техникой.»

| вид | idle | walk | attack | punch | take_damage | loot |
|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| front | ![](media/heroes/engineer/down/idle.gif) | ![](media/heroes/engineer/down/walk.gif) | ![](media/heroes/engineer/down/attack.gif) | ![](media/heroes/engineer/down/punch.gif) | ![](media/heroes/engineer/down/take_damage.gif) | ![](media/heroes/engineer/down/loot.gif) |
| back | ![](media/heroes/engineer/up/idle.gif) | ![](media/heroes/engineer/up/walk.gif) | ![](media/heroes/engineer/up/attack.gif) | ![](media/heroes/engineer/up/punch.gif) | ![](media/heroes/engineer/up/take_damage.gif) | ![](media/heroes/engineer/up/loot.gif) |
| side | ![](media/heroes/engineer/side/idle.gif) | ![](media/heroes/engineer/side/walk.gif) | ![](media/heroes/engineer/side/attack.gif) | ![](media/heroes/engineer/side/punch.gif) | ![](media/heroes/engineer/side/take_damage.gif) | ![](media/heroes/engineer/side/loot.gif) |

---

## Заметки

- Все **6 действий × 3 вида** подключены в коде (`hero.Clip(action, facing)`),
  раскладка `assets/sprites/heroes/<id>/<down|up|side>/<action>/`.
- В галерее (`CHOOSE HEROES` → showcase) стрелки **вверх/вниз** переключают вид
  (front / back / side → / side ←), **влево/вправо** — героя.
- **side** — настоящий профиль (лицом вправо), нарисован параметрически из
  палитры героя (`spritegen.buildSide`); взгляд влево = тот же профиль, зеркально.
- Эффекты умений (Раскол, Огненный шар и т.д.) — отдельная витрина: `skills.md`.
