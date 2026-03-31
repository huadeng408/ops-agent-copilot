from pathlib import Path

from app.core.config import Settings


def test_openai_api_key_can_be_loaded_from_auth_json(tmp_path: Path) -> None:
    auth_file = tmp_path / 'auth.json'
    auth_file.write_text('{"OPENAI_API_KEY":"sk-relay-test"}', encoding='utf-8')

    settings = Settings(
        _env_file=None,
        OPENAI_BASE_URL='https://codex.ai02.cn/v1',
        OPENAI_API_KEY='your_openai_api_key_here',
        OPENAI_AUTH_FILE=str(auth_file),
    )

    assert settings.openai_api_key == 'sk-relay-test'
    assert settings.has_real_openai_api_key is True


def test_openai_auth_json_overrides_env_key_when_explicitly_configured(tmp_path: Path) -> None:
    auth_file = tmp_path / 'auth.json'
    auth_file.write_text('{"OPENAI_API_KEY":"sk-relay-override"}', encoding='utf-8')

    settings = Settings(
        _env_file=None,
        OPENAI_BASE_URL='https://codex.ai02.cn/v1',
        OPENAI_API_KEY='sk-env-should-be-overridden',
        OPENAI_AUTH_FILE=str(auth_file),
    )

    assert settings.openai_api_key == 'sk-relay-override'
