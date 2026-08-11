package entity

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/art"
	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/skill"
)

const PlayerRadius = 7

type Player struct {
	Pos    engine.Vec2
	Stats  combat.Stats
	Weapon item.Weapon
	Skill  skill.Skill

	cd     int         // откат текущего умения
	aim    engine.Vec2 // направление прицела
	sprite *ebiten.Image
}

func NewPlayer(pos engine.Vec2) *Player {
	st := combat.Stats{
		MaxLife: 100, Life: 100,
		MaxMana: 50, Mana: 50,
		Armour:    30,
		MoveSpeed: 2.4,
		LifeRegen: 3,
		ManaRegen: 8,
	}
	st.Resist[combat.Fire] = 0.20
	st.Resist[combat.Cold] = 0.20
	st.Resist[combat.Lightning] = 0.20
	return &Player{
		Pos:    pos,
		Stats:  st,
		Weapon: item.StarterWeapon(),
		Skill:  skill.Fireball(),
		aim:    engine.Vec2{X: 1},
		sprite: art.Player(),
	}
}

func (p *Player) Update(w World) {
	p.Stats.RegenTick()
	if p.cd > 0 {
		p.cd--
	}

	// Движение.
	var dir engine.Vec2
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		dir.Y--
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		dir.Y++
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		dir.X--
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		dir.X++
	}
	p.Pos = p.Pos.Add(dir.Normalized().Scale(p.Stats.MoveSpeed))

	// Прицел — на курсор (в мировых координатах).
	toCursor := w.CursorWorld().Sub(p.Pos)
	if toCursor.Len() > 0 {
		p.aim = toCursor.Normalized()
	}

	// Применение умения.
	casting := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		ebiten.IsKeyPressed(ebiten.KeySpace)
	if casting && p.cd == 0 && p.Stats.SpendMana(p.Skill.Mana) {
		// Урон умения складывается с уроном оружия того же типа умения.
		dmg := combat.Damage{
			Type:   p.Skill.Damage.Type,
			Amount: p.Skill.Damage.Amount + p.Weapon.Damage.Amount,
		}
		muzzle := p.Pos.Add(p.aim.Scale(PlayerRadius + 4))
		w.AddProjectile(NewProjectile(muzzle, p.aim, dmg, p.Skill.ProjectileSpeed))
		p.cd = p.Skill.Cooldown
	}
}

func (p *Player) Alive() bool { return p.Stats.Alive() }

func (p *Player) Draw(screen *ebiten.Image, cam *engine.Camera) {
	drawSprite(screen, p.sprite, cam, p.Pos, 0)
}
