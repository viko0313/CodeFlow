package redis

type RedisConfig struct {
	Host          string
	Port          int
	DB            int
	SessionPrefix string
}
