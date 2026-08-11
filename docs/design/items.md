# Предметы — визуальная витрина

Статус: 🟡 базовые иконки готовы (18 баз); редкости/аффиксы — post-MVP
Связь: `../mechanics/items.md` (система лута), `art-direction.md` (как рисуем),
`to-draw.md` (бэклог)

Каталог **иконок базовых предметов** для панели экипировки. Иконки **статичные**
(без анимации), поэтому здесь обычные PNG, а не GIF. Что за предметы, сокеты,
редкости и дроп — в `../mechanics/items.md`.

**Материал кодирует тип защиты** (`../mechanics/damage-and-defense.md`):
**сталь → Броня** (Сила) · **кожа → Уклонение** (Ловкость) · **ткань+руна →
Энергощит** (Интеллект). Так по виду сразу ясен защитный профиль базы.

> Исходники: `assets/sprites/items/…` (256×256). В витрине — ресайз до 128px в
> `media/items/` (magick). Иконки рисует процедурный Go-генератор (как герои и
> скиллы), живущий вне игрового кода.

---

## 🛡️ Броня (4 слота × 3 типа защиты)

| Слот | Сталь — Броня | Кожа — Уклонение | Ткань — Энергощит |
|------|:-------------:|:----------------:|:-----------------:|
| **Нагрудник** | ![](media/items/chest/plate.png) | ![](media/items/chest/leather.png) | ![](media/items/chest/cloth.png) |
| **Шлем** | ![](media/items/helmet/plate.png) | ![](media/items/helmet/leather.png) | ![](media/items/helmet/cloth.png) |
| **Перчатки** | ![](media/items/gloves/plate.png) | ![](media/items/gloves/leather.png) | ![](media/items/gloves/cloth.png) |
| **Сапоги** | ![](media/items/boots/plate.png) | ![](media/items/boots/leather.png) | ![](media/items/boots/cloth.png) |

Нагрудник — 4 сокета, остальное по 2 (число на дропе случайно, см. `../mechanics/items.md`).

## ⚔️ Оружие (по одному на класс)

| Меч · Рыцарь | Топор · Дикарь | Лук · Следопыт | Кинжалы · Ассасин | Посох · Маг | Ключ · Инженер |
|:---:|:---:|:---:|:---:|:---:|:---:|
| ![](media/items/weapon/sword.png) | ![](media/items/weapon/axe.png) | ![](media/items/weapon/bow.png) | ![](media/items/weapon/daggers.png) | ![](media/items/weapon/staff.png) | ![](media/items/weapon/wrench.png) |

Оружие даёт базовый урон атак и скорость (числа — `../progression/character-progression.md`).

---

## Впереди (ещё не нарисованы)

- **Иконки скиллов и гемов** (для сокетов/панели) — `to-draw.md`.
- **Редкости/аффиксы** (Магический/Редкий/Уникальный) и их подсветка — post-MVP.
