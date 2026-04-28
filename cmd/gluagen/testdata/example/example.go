package example

import "fmt"

//glua:module name=game

//glua:bind
type Player struct {
	Name string `lua:"name"`
	HP   int    `lua:"hp"`
	MP   int    `lua:"mp"`
}

//glua:bind
func NewPlayer(name string, hp int) *Player {
	return &Player{Name: name, HP: hp, MP: 50}
}

//glua:bind
func (p *Player) Attack(damage int) int {
	p.HP -= damage
	if p.HP < 0 {
		p.HP = 0
	}
	return p.HP
}

//glua:bind
func (p *Player) Heal(amount int) int {
	p.HP += amount
	return p.HP
}

//glua:bind
func (p *Player) String() string {
	return fmt.Sprintf("Player{%s, HP=%d}", p.Name, p.HP)
}

//glua:bind
func CalculateDamage(atk, def int) int {
	dmg := atk - def
	if dmg < 1 {
		return 1
	}
	return dmg
}
