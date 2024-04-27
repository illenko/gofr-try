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

type cardConfig struct {
	Id    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Skin  string    `json:"skin"`
}

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())

	app.AddHTTPService("core-banking-system", app.Config.Get("CORE_BANKING_SYSTEM_URL"))

	app.GET("/cards", func(c *gofr.Context) (interface{}, error) {
		externalCards, err := getCards(c)
		if err != nil {
			return nil, err
		}

		configs, err := getConfigs(c)
		if err != nil {
			return nil, err
		}

		cardResponse := toCardResponse(externalCards, configs)

		return cardResponse, nil
	})

	app.PUT("/card-configs/{cardId}", updateCardConfig)

	app.Run()
}

func updateCardConfig(c *gofr.Context) (interface{}, error) {
	var cfg cardConfig

	err := c.Bind(&cfg)

	id := uuid.MustParse(c.PathParam("cardId"))

	if err != nil {
		return nil, err
	}

	rows, err := c.SQL.QueryContext(c, "SELECT id, title, skin FROM card_config where id = $1", id)
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		c.Info("Updating existing config")
		_, err := c.SQL.ExecContext(c, "UPDATE card_config SET title = $1, skin = $2 where id = $3", cfg.Title, cfg.Skin, id)
		if err != nil {
			return nil, err
		}
	} else {
		c.Info("Inserting new config")
		_, err := c.SQL.ExecContext(c, "INSERT INTO card_config (id, title, skin) VALUES ($1, $2, $3)", id, cfg.Title, cfg.Skin)
		if err != nil {
			return nil, err
		}
	}

	return cardConfig{
		Id:    id,
		Title: cfg.Title,
		Skin:  cfg.Skin,
	}, nil
}

func getConfigs(c *gofr.Context) (map[uuid.UUID]*cardConfig, error) {
	rows, err := c.SQL.QueryContext(c, "SELECT id, title, skin FROM card_config")
	if err != nil {
		return nil, err
	}

	configs := make(map[uuid.UUID]*cardConfig)

	for rows.Next() {
		var cfg cardConfig
		if err := rows.Scan(&cfg.Id, &cfg.Title, &cfg.Skin); err != nil {
			return nil, err
		}

		configs[cfg.Id] = &cfg
	}
	c.Info("Retrieved configs from database")

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

func toCardResponse(externalCards []externalCard, configs map[uuid.UUID]*cardConfig) (cards []card) {
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
