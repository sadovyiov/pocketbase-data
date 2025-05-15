package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/lmittmann/tint"
	"github.com/urfave/cli/v2"
	_ "github.com/urfave/cli/v2"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AuthRequestBody struct {
	Identity string `json:"identity"`
	Password string `json:"password"`
}

type Admin struct {
	ID      string `json:"id"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Email   string `json:"email"`
	Avatar  int    `json:"avatar"`
}

type AdminResponseBody struct {
	Token string `json:"token"`
	Admin Admin  `json:"admin"`
}

type Schema struct {
	Fields []struct {
		Name  string `yaml:"name"`
		Type  string `yaml:"type"`
		Value string `yaml:"value,omitempty"`
	} `yaml:"fields"`
}

type Config struct {
	URL      string `yaml:"url" env:"URL"`
	Email    string `yaml:"email" env:"EMAIL"`
	Password string `yaml:"password" env:"PASSWORD"`
}

var cfg Config

func init() {
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: time.DateTime,
	})))
}

type Record map[string]interface{}

const (
	Fake      = "fake"
	Dependent = "dependent"
	Relation  = "relation"
	Custom    = "custom"
)

func main() {
	defer duration(track("foo"))

	app := &cli.App{
		Name:     "pbd",
		Version:  "v1.0.0",
		Compiled: time.Now(),
		Authors: []*cli.Author{
			&cli.Author{
				Name:  "Oleksandr Sadovyi",
				Email: "sadovyiov@gmail.com",
			},
		},
		HelpName: "PBD",
		Usage:    "Seed the database with records",
		Commands: []*cli.Command{
			{
				Name:  "seed",
				Usage: "seed the database with records",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Required: true,
						Usage:    "Load configuration from `FILE`",
					},
					&cli.StringFlag{
						Name:     "collection",
						Usage:    "Collection to seed",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "schema",
						Usage:    "Load schema from `FILE`",
						Required: true,
					},
					&cli.IntFlag{
						Name:        "count",
						Usage:       "Number of records to seed",
						Value:       10,
						DefaultText: "10",
					},
					// TODO
					&cli.IntFlag{
						Name:  "batch",
						Usage: "Number of records to seed in a batch",
					},
				},
				Action: func(cCtx *cli.Context) error {
					err := cleanenv.ReadConfig(cCtx.String("config"), &cfg)
					if err != nil {
						slog.Error("reading config", "error", err)
					}

					var schema Schema

					err = cleanenv.ReadConfig(cCtx.String("schema"), &schema)
					if err != nil {
						slog.Error("reading schema", "error", err)
					}

					authResponse, err := authenticateAdmin(cfg.Email, cfg.Password)
					if err != nil {
						log.Fatalf("Authentication failed: %v", err)
					}

					token := authResponse.Token

					items := make(chan Record)

					slog.Info("batch size", "batch", cCtx.Int("batch"))

					go func() {
						for i := 0; i < cCtx.Int("count"); i++ {
							r, err := fakeRecord(schema, token)
							if err != nil {
								slog.Error("generating fake record", "error", err)
								return
							}

							items <- r
						}

						close(items)
					}()

					records := make(chan Record)

					go func() {
						for item := range items {
							r, err := createRecord(item, cCtx.String("collection"), token)
							if err != nil {
								slog.Error("creating record", "error", err)
								return
							}

							records <- r
						}

						close(records)
					}()

					for r := range records {
						slog.Info("record created", "record", r)
					}

					return nil
				},
			},
			{
				Name:  "import",
				Usage: "import records from a file",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Required: true,
						Usage:    "Load configuration from `FILE`",
					},
					&cli.StringFlag{
						Name:     "collection",
						Usage:    "Collection to seed",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "file",
						Usage:    "Load records from `FILE`",
						Required: true,
					},
				},
				Action: func(cCtx *cli.Context) error {
					err := cleanenv.ReadConfig(cCtx.String("config"), &cfg)
					if err != nil {
						slog.Error("Failed to read config", "error", err)
					}

					filePath := cCtx.String("file")
					fileExt := strings.ToLower(filepath.Ext(filePath))

					authResponse, err := authenticateAdmin(cfg.Email, cfg.Password)
					if err != nil {
						log.Fatalf("Authentication failed: %v", err)
					}

					token := authResponse.Token

					var items []Record

					switch fileExt {
					case ".json":
						items = ReadJson(filePath)
						if err != nil {
							slog.Error("reading JSON records", "error", err)
							return err
						}
					case ".csv":
						items = ReadCsv(filePath)
					default:
						return fmt.Errorf("unsupported file type: %s", fileExt)
					}

					slog.Info("importing records", "count", len(items))

					records := make(chan Record)

					go func() {
						for _, item := range items {
							slog.Debug("processing record", "record", item)

							r, err := createRecord(item, cCtx.String("collection"), token)
							if err != nil {
								slog.Error("creating record", "error", err)
								return
							}

							records <- r
						}

						close(records)
					}()

					for r := range records {
						slog.Info("record created", "record", r)
					}

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("running app", "error", err)
	}
}

func fakeRecord(schema Schema, token string) (Record, error) {
	r := Record{}

	err := gofakeit.Seed(0)
	if err != nil {
		return Record{}, err
	}

	for _, field := range schema.Fields {
		if field.Type == Fake {
			r[field.Name], _ = gofakeit.Generate(field.Value)
		}

		if field.Type == Dependent {
			r[field.Name] = r[field.Value]
		}

		if field.Type == Custom {
			r[field.Name] = field.Value
		}

		if field.Type == Relation {
			rr, err := getRandomRecord(field.Value, token)
			slog.Info("random record", "record", rr)
			if err != nil {
				return Record{}, err
			}

			r[field.Name] = rr["id"]
		}
	}

	return r, nil
}

func authenticateAdmin(email, password string) (AdminResponseBody, error) {
	reqBody := AuthRequestBody{Identity: email, Password: password}
	var respBody AdminResponseBody

	err := sendRequest("POST", "/collections/_superusers/auth-with-password", "", reqBody, &respBody)
	if err != nil {
		return AdminResponseBody{}, err
	}

	return respBody, nil
}

func createRecord(record interface{}, col string, token string) (Record, error) {
	r := Record{}

	err := sendRequest("POST", "/collections/"+col+"/records", token, record, &r)
	if err != nil {
		return r, err
	}

	return r, nil
}

func getRandomRecord(col string, token string) (Record, error) {
	var respBody struct {
		Items      []Record `json:"items"`
		Page       int      `json:"page"`
		PerPage    int      `json:"perPage"`
		TotalItems int      `json:"totalItems"`
		TotalPages int      `json:"totalPages"`
	}

	err := sendRequest("GET", "/collections/"+col+"/records?perPage=1&skipTotal=true&sort=@random&fields=id", token, nil, &respBody)
	if err != nil {
		return nil, err
	}

	if len(respBody.Items) == 0 {
		return nil, fmt.Errorf("no records found")
	}

	return respBody.Items[0], nil
}

func sendRequest(method, url string, token string, reqBody interface{}, respBody interface{}) error {
	var jsonData []byte

	var err error
	if reqBody != nil {
		jsonData, err = json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %v", err)
		}
	}

	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s%s", cfg.URL, "api", url), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("received non-200 status code: %d, body: %s", resp.StatusCode, body)
	}
	if respBody != nil {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}
		err = json.Unmarshal(body, respBody)
		if err != nil {
			return fmt.Errorf("failed to unmarshal response body: %v", err)
		}
	}

	return nil
}

func track(msg string) (string, time.Time) {
	return msg, time.Now()
}

func duration(msg string, start time.Time) {
	slog.Info("execution time", msg, time.Since(start))
}

func ReadJson(filePath string) []Record {
	file, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error("reading JSON file", "error", err)
		return nil
	}

	var items []Record
	if err := json.Unmarshal(file, &items); err != nil {
		slog.Error("unmarshaling JSON", "error", err)
		return nil
	}

	return items
}

func ReadCsv(filePath string) []Record {
	csvFile, err := os.OpenFile(filePath, os.O_RDWR, os.ModePerm)
	if err != nil {
		slog.Error("opening CSV file", "error", err)
		return nil
	}
	defer csvFile.Close()

	var rawRecords [][]string

	reader := csv.NewReader(csvFile)
	rawRecords, err = reader.ReadAll()
	if err != nil {
		slog.Error("reading CSV", "error", err)
		return nil
	}

	if len(rawRecords) < 2 {
		slog.Error("CSV file must have headers and at least one record")
		return nil
	}

	headers := rawRecords[0]
	records := make([]Record, 0, len(rawRecords)-1)

	for _, row := range rawRecords[1:] {
		record := make(Record)
		for i, value := range row {
			if i < len(headers) {
				record[headers[i]] = value
			}
		}
		records = append(records, record)
	}

	return records
}
