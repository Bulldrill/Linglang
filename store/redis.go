package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"linlang-go/core"
)

// RedisStore almacena cada vector LinLang como un Redis Hash.
//
// Esquema de claves:
//
//	linlang:{SpaceName}:{id}  → Hash con un campo por dimensión
//
// Índice del espacio (para iterar todos los vectores):
//
//	linlang:{SpaceName}:_ids  → Redis Set con todos los ids del espacio
//
// DSN: redis://[:password@]host:port[/db]
// Ejemplo: redis://localhost:6379/0
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

func OpenRedis(dsn string) (*RedisStore, error) {
	// Convertir DSN estilo URI a opciones go-redis
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, fmt.Errorf("redis: DSN inválido '%s': %w", dsn, err)
	}
	client := redis.NewClient(opt)
	ctx := context.Background()
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("redis: ping falló: %w", err)
	}
	return &RedisStore{client: client, ctx: ctx}, nil
}

// vectorKey devuelve la clave del Hash para un vector.
func vectorKey(spaceName string, id float64) string {
	return fmt.Sprintf("linlang:%s:%.0f", spaceName, id)
}

// indexKey devuelve la clave del Set de IDs para un espacio.
func indexKey(spaceName string) string {
	return fmt.Sprintf("linlang:%s:_ids", spaceName)
}

func (r *RedisStore) Upsert(space *core.Space, values []float64) error {
	id := values[0]
	key := vectorKey(space.Name, id)

	// Construir el hash: dim → valor como string
	fields := make([]any, 0, len(space.Dimensions)*2)
	for i, dim := range space.Dimensions {
		fields = append(fields, dim, strconv.FormatFloat(values[i], 'f', -1, 64))
	}

	pipe := r.client.Pipeline()
	pipe.HSet(r.ctx, key, fields...)
	pipe.SAdd(r.ctx, indexKey(space.Name), id)
	_, err := pipe.Exec(r.ctx)
	return err
}

func (r *RedisStore) Query(space *core.Space, filter func([]float64) bool) ([]*core.Vector, error) {
	// Obtener todos los IDs del espacio
	ids, err := r.client.SMembers(r.ctx, indexKey(space.Name)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: SMEMBERS %s: %w", indexKey(space.Name), err)
	}

	var result []*core.Vector
	for _, idStr := range ids {
		id, err := strconv.ParseFloat(strings.TrimSpace(idStr), 64)
		if err != nil {
			continue
		}
		key := vectorKey(space.Name, id)

		// Leer los campos del hash en orden de dimensiones
		vals := make([]float64, len(space.Dimensions))
		for i, dim := range space.Dimensions {
			raw, err := r.client.HGet(r.ctx, key, dim).Result()
			if err != nil {
				if err == redis.Nil {
					vals[i] = 0
					continue
				}
				return nil, fmt.Errorf("redis: HGET %s %s: %w", key, dim, err)
			}
			f, _ := strconv.ParseFloat(raw, 64)
			vals[i] = f
		}

		if filter == nil || filter(vals) {
			result = append(result, core.NewVector(space, vals))
		}
	}
	return result, nil
}

func (r *RedisStore) Delete(space *core.Space, id float64) error {
	key := vectorKey(space.Name, id)
	pipe := r.client.Pipeline()
	pipe.Del(r.ctx, key)
	pipe.SRem(r.ctx, indexKey(space.Name), id)
	_, err := pipe.Exec(r.ctx)
	return err
}

func (r *RedisStore) Close() error { return r.client.Close() }
