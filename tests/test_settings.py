from app.core.config import Settings


def test_remote_openai_base_url_uses_system_proxy_by_default() -> None:
    settings = Settings(
        _env_file=None,
        OPENAI_BASE_URL='https://api.openai.com/v1',
        OPENAI_API_KEY='sk-test',
    )

    assert settings.resolved_openai_trust_env is True


def test_local_openai_base_url_skips_system_proxy_by_default() -> None:
    settings = Settings(
        _env_file=None,
        OPENAI_BASE_URL='http://127.0.0.1:8001/v1',
        OPENAI_API_KEY='sk-test',
    )

    assert settings.resolved_openai_trust_env is False


def test_openai_trust_env_can_be_overridden_explicitly() -> None:
    settings = Settings(
        _env_file=None,
        OPENAI_BASE_URL='https://api.openai.com/v1',
        OPENAI_API_KEY='sk-test',
        OPENAI_TRUST_ENV='false',
    )

    assert settings.resolved_openai_trust_env is False
