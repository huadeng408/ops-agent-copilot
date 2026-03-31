from functools import lru_cache
import json
from pathlib import Path
from urllib.parse import urlparse

from pydantic import Field
from pydantic import model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file='.env',
        env_file_encoding='utf-8',
        case_sensitive=False,
        extra='ignore',
    )

    app_env: str = Field(default='dev', alias='APP_ENV')
    app_name: str = Field(default='ops-agent-copilot', alias='APP_NAME')
    host: str = Field(default='0.0.0.0', alias='HOST')
    port: int = Field(default=8000, alias='PORT')

    mysql_host: str = Field(default='127.0.0.1', alias='MYSQL_HOST')
    mysql_port: int = Field(default=3306, alias='MYSQL_PORT')
    mysql_db: str = Field(default='ops_agent', alias='MYSQL_DB')
    mysql_user: str = Field(default='root', alias='MYSQL_USER')
    mysql_password: str = Field(default='123456', alias='MYSQL_PASSWORD')

    redis_host: str = Field(default='127.0.0.1', alias='REDIS_HOST')
    redis_port: int = Field(default=6379, alias='REDIS_PORT')

    openai_base_url: str = Field(default='https://api.moonshot.cn/v1', alias='OPENAI_BASE_URL')
    openai_api_key: str = Field(default='sk-xxx', alias='OPENAI_API_KEY')
    openai_model: str = Field(default='kimi-k2-0905-preview', alias='OPENAI_MODEL')
    openai_trust_env: bool | None = Field(default=None, alias='OPENAI_TRUST_ENV')
    openai_auth_file: str | None = Field(default=None, alias='OPENAI_AUTH_FILE')

    log_level: str = Field(default='INFO', alias='LOG_LEVEL')
    database_url: str | None = Field(default=None, alias='DATABASE_URL')
    redis_url: str | None = Field(default=None, alias='REDIS_URL')
    agent_runtime_mode: str = Field(default='auto', alias='AGENT_RUNTIME_MODE')
    metrics_enabled: bool = Field(default=True, alias='METRICS_ENABLED')
    otel_enabled: bool = Field(default=False, alias='OTEL_ENABLED')
    otel_service_name: str = Field(default='ops-agent-copilot', alias='OTEL_SERVICE_NAME')
    otel_exporter_otlp_endpoint: str = Field(default='http://127.0.0.1:4318', alias='OTEL_EXPORTER_OTLP_ENDPOINT')
    keep_recent_message_count: int = 8
    readonly_sql_limit: int = 200

    @property
    def sqlalchemy_database_url(self) -> str:
        if self.database_url:
            return self.database_url
        return (
            f'mysql+asyncmy://{self.mysql_user}:{self.mysql_password}'
            f'@{self.mysql_host}:{self.mysql_port}/{self.mysql_db}?charset=utf8mb4'
        )

    @property
    def cache_url(self) -> str:
        if self.redis_url:
            return self.redis_url
        return f'redis://{self.redis_host}:{self.redis_port}/0'

    @property
    def has_real_openai_api_key(self) -> bool:
        api_key = self.openai_api_key.strip()
        placeholders = {'', 'sk-test', 'sk-xxx', 'your_openai_api_key_here', 'your-openai-api-key'}
        return api_key not in placeholders

    @property
    def resolved_openai_trust_env(self) -> bool:
        if self.openai_trust_env is not None:
            return self.openai_trust_env

        hostname = (urlparse(self.openai_base_url).hostname or '').lower()
        return hostname not in {'localhost', '127.0.0.1', '::1'}

    @model_validator(mode='after')
    def _load_openai_auth_file(self) -> 'Settings':
        loaded_key = self._read_openai_api_key_from_auth_file()
        if loaded_key:
            self.openai_api_key = loaded_key
        return self

    def _read_openai_api_key_from_auth_file(self) -> str | None:
        candidates: list[Path] = []
        if self.openai_auth_file:
            candidates.append(Path(self.openai_auth_file))
        candidates.append(Path('auth.json'))

        for candidate in candidates:
            path = candidate if candidate.is_absolute() else Path.cwd() / candidate
            if not path.exists() or not path.is_file():
                continue

            try:
                payload = json.loads(path.read_text(encoding='utf-8'))
            except (OSError, json.JSONDecodeError):
                continue

            if isinstance(payload, dict):
                key = payload.get('OPENAI_API_KEY')
                if isinstance(key, str) and key.strip():
                    return key.strip()

        return None


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
