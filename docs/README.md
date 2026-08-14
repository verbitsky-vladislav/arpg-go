# Документация

Каноничные описания форматов и правил игры. Каждый файл отвечает за один
предмет и написан по одному правилу: сначала **почему так**, потом таблица
полей. Если код и документ разошлись — прав документ, а код ошибка (или
наоборот, но тогда правится и документ).

## Мир

| | о чём |
|---|---|
| [worldgen/worldgen.spec.md](worldgen/worldgen.spec.md) | что на самом деле лежит в купленном тайлсете и что из этого следует для генератора |
| [worldgen/autotiling.md](worldgen/autotiling.md) | угловая dual-grid разметка: почему не blob-47 и как считается стык |
| [worldgen/layers_and_plateau.md](worldgen/layers_and_plateau.md) | три уровня высоты, обрыв, лестницы, лианы и тени |
| [worldgen/manifest.schema.md](worldgen/manifest.schema.md) | манифест биома: чем тайл является, какие роли обязательны |
| [worldgen/map_format.md](worldgen/map_format.md) | map_format v1 — карта как данные: тайлы, слои, пропсы, физика |
| [worldgen/tileset-onboarding.md](worldgen/tileset-onboarding.md) | пошагово: как подключить новый биом с нуля |

## Герой

| | о чём |
|---|---|
| [character/character.md](character/character.md) | тела и лоадауты, автомат состояний, деградация клипов, удар сектором |
| [character/combat.md](character/combat.md) | откуда берётся урон: база героя, вещь в руке, виды урона, кража силы |
| [character/progress.md](character/progress.md) | уровень существа выводится из его чисел; опыт и цена уровня |

## Мобы

| | о чём |
|---|---|
| [mobs/species.md](mobs/species.md) | звери: виды, повадки, взросление, среда обитания |
| [mobs/spawn.md](mobs/spawn.md) | население живёт вокруг игрока и не копится за спиной |
| [mobs/enemies.md](mobs/enemies.md) | враги и боссы: тип и тир, сила, дроп |
| [mobs/enemies_ai.md](mobs/enemies_ai.md) | восприятие (зрение, слух, боль), память, отряд |
| [mobs/enemies_spawn.md](mobs/enemies_spawn.md) | бюджет опасности вместо числа голов |
| [mobs/loot.md](mobs/loot.md) | что падает, как долетает до земли и до сумки |

## Забег

| | о чём |
|---|---|
| [save.md](save.md) | три персонажа, мир на диске, что именно сохраняется и почему не сид |
| [sound.md](sound.md) | звук как код: синтез эффектов и музыки, банк, правила проигрывания |

## Картинки

[media/](media) — витрина для [README](../README.md). Собирается командой
`go run ./tools/showcase` из тех же ассетов, которыми играет игра, поэтому
устареть молча не может.
