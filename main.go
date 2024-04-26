package main

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/illenko/gofr-try/migrations"
	"github.com/redis/go-redis/v9"
	"gofr.dev/pkg/gofr"
	"io"
	"time"
)

const (
	defaultTitle = "My favorite card"
	defaultSkin  = "default_skin"
)

type externalCard struct {
	Id              uuid.UUID `json:"id"`
	Account         string    `json:"account"`
	Number          string    `json:"number"`
	ExpirationMonth string    `json:"expirationMonth"`
	ExpirationYear  string    `json:"expirationYear"`
	Currency        string    `json:"currency"`
	Balance         float64   `json:"balance"`
}

type card struct {
	Id              uuid.UUID `json:"id"`
	Number          string    `json:"number"`
	ExpirationMonth string    `json:"expirationMonth"`
	ExpirationYear  string    `json:"expirationYear"`
	Currency        string    `json:"currency"`
	Title           string    `json:"title"`
	Skin            string    `json:"skin"`
}

type config struct {
	Id    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Skin  string    `json:"skin"`
}

func (u *config) Create(c *gofr.Context) (interface{}, error) {
	var cfg config

	err := c.Bind(&cfg)

	if err != nil {
		return nil, err
	}

	_, err = c.SQL.ExecContext(c, "INSERT INTO config (Id, Title, Skin) VALUES ($1, $2, $3)", cfg.Id, cfg.Title, cfg.Skin)

	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())

	app.AddRESTHandlers(&config{})

	app.AddHTTPService("core-banking-system", "http://localhost:8081")

	app.GET("/cards", func(c *gofr.Context) (interface{}, error) {
		externalCards, err := getCards(c)
		if err != nil {
			return nil, err
		}

		configs, err := getConfigs(c, err)
		if err != nil {
			return nil, err
		}

		cardResponse := toCardResponse(externalCards, configs)

		return cardResponse, nil
	})

	app.Run()
}

func getConfigs(c *gofr.Context, err error) (map[uuid.UUID]*config, error) {
	rows, err := c.SQL.QueryContext(c, "SELECT Id, Title, Skin FROM config")
	if err != nil {
		return nil, err
	}

	var configs map[uuid.UUID]*config

	for rows.Next() {
		var cfg config
		if err := rows.Scan(&cfg.Id, &cfg.Title, &cfg.Skin); err != nil {
			return nil, err
		}

		configs[cfg.Id] = &cfg
	}
	return configs, nil
}

func getCards(c *gofr.Context) ([]externalCard, error) {
	cached, err := c.Redis.Get(c.Context, "cards").Result()
	var body []byte
	if errors.Is(err, redis.Nil) {
		cbs := c.GetHTTPService("core-banking-system")
		c.Info("External card response not found in cache, executing request to core banking system...")
		resp, err := cbs.Get(c, "api/v1/cards", nil)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		c.Redis.Set(c.Context, "cards", body, 10*time.Second)
	} else {
		c.Info("External card response retrieved from cache.")
		body = []byte(cached)
	}

	var externalCards []externalCard
	err = json.Unmarshal(body, &externalCards)
	if err != nil {
		return nil, err
	}
	return externalCards, nil
}

func toCardResponse(externalCards []externalCard, configs map[uuid.UUID]*config) (cards []card) {
	for _, c := range externalCards {
		var title, skin string
		cfg := configs[c.Id]
		if cfg == nil {
			title = defaultTitle
			skin = defaultSkin
		} else {
			title = cfg.Title
			skin = cfg.Skin
		}
		cards = append(cards, card{
			Id:              c.Id,
			Number:          c.Number,
			ExpirationMonth: c.ExpirationMonth,
			ExpirationYear:  c.ExpirationYear,
			Currency:        c.Currency,
			Title:           title,
			Skin:            skin,
		})
	}
	return
}
