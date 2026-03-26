"""Tests for PDF extraction OCR selection helpers."""


from refloom_worker.pdf_extractor import (
    _load_ocr_settings,
    _non_space_len,
    _ocr_cache_get,
    _ocr_cache_key,
    _ocr_cache_put,
    _preferred_ocr_text,
    _sample_page_numbers,
    close_cached_doc,
)


def test_preferred_ocr_text_keeps_fast_result_when_retry_is_empty():
    assert _preferred_ocr_text("短い見出し", "") == "短い見出し"


def test_preferred_ocr_text_keeps_fast_result_when_retry_is_not_better():
    assert _preferred_ocr_text("図1", "図A") == "図1"


def test_preferred_ocr_text_uses_retry_when_it_recovers_more_text():
    assert _preferred_ocr_text("短文", "短文の詳細です") == "短文の詳細です"


def test_non_space_len_ignores_whitespace():
    assert _non_space_len(" A \n B\t") == 2


def test_load_ocr_settings_uses_env(monkeypatch):
    monkeypatch.setenv("REFLOOM_OCR_FAST_SCALE", "1.25")
    monkeypatch.setenv("REFLOOM_OCR_RETRY_MIN_CHARS", "30")

    settings = _load_ocr_settings()

    assert settings.fast_render_scale == 1.25
    assert settings.retry_min_chars == 30


def test_load_ocr_settings_falls_back_for_invalid_env(monkeypatch):
    monkeypatch.setenv("REFLOOM_OCR_FAST_SCALE", "invalid")
    monkeypatch.setenv("REFLOOM_OCR_RETRY_MIN_CHARS", "invalid")

    settings = _load_ocr_settings()

    assert settings.fast_render_scale == 1.5
    assert settings.retry_min_chars == 50


def test_sample_page_numbers_uses_first_middle_last_without_duplicates():
    assert _sample_page_numbers(1) == [1]
    assert _sample_page_numbers(2) == [1, 2]
    assert _sample_page_numbers(10) == [1, 5, 10]


# --- OCR cache tests ---


def test_ocr_cache_key_is_deterministic():
    k1 = _ocr_cache_key("abc", 1, 2.0, 0)
    k2 = _ocr_cache_key("abc", 1, 2.0, 0)
    assert k1 == k2


def test_ocr_cache_key_differs_for_different_params():
    k1 = _ocr_cache_key("abc", 1, 2.0, 0)
    k2 = _ocr_cache_key("abc", 2, 2.0, 0)
    k3 = _ocr_cache_key("abc", 1, 1.5, 0)
    k4 = _ocr_cache_key("xyz", 1, 2.0, 0)
    assert len({k1, k2, k3, k4}) == 4


def test_ocr_cache_put_get_roundtrip(monkeypatch, tmp_path):
    monkeypatch.setattr("refloom_worker.pdf_extractor._ocr_cache_dir", lambda: tmp_path)
    _ocr_cache_put("hash1", 5, 2.0, 0, "OCR結果テキスト")
    result = _ocr_cache_get("hash1", 5, 2.0, 0)
    assert result == "OCR結果テキスト"


def test_ocr_cache_get_returns_none_for_missing(monkeypatch, tmp_path):
    monkeypatch.setattr("refloom_worker.pdf_extractor._ocr_cache_dir", lambda: tmp_path)
    assert _ocr_cache_get("hash1", 1, 2.0, 0) is None


def test_ocr_cache_get_returns_none_for_corrupt_file(monkeypatch, tmp_path):
    monkeypatch.setattr("refloom_worker.pdf_extractor._ocr_cache_dir", lambda: tmp_path)
    key = _ocr_cache_key("hash1", 1, 2.0, 0)
    (tmp_path / f"{key}.json").write_text("not valid json")
    assert _ocr_cache_get("hash1", 1, 2.0, 0) is None


def test_ocr_cache_get_returns_none_when_no_file_hash():
    assert _ocr_cache_get(None, 1, 2.0, 0) is None
    assert _ocr_cache_get("", 1, 2.0, 0) is None


def test_ocr_cache_put_skips_when_no_file_hash(tmp_path):
    # Should not raise or create files
    _ocr_cache_put(None, 1, 2.0, 0, "text")
    _ocr_cache_put("", 1, 2.0, 0, "text")


def test_ocr_cache_put_atomic_no_tmp_file_remains(monkeypatch, tmp_path):
    monkeypatch.setattr("refloom_worker.pdf_extractor._ocr_cache_dir", lambda: tmp_path)
    _ocr_cache_put("hash1", 1, 2.0, 0, "text")
    tmp_files = list(tmp_path.glob("*.tmp"))
    assert len(tmp_files) == 0
    json_files = list(tmp_path.glob("*.json"))
    assert len(json_files) == 1


def test_close_cached_doc_resets_state():
    # Should not raise even when no doc is cached
    close_cached_doc()
    close_cached_doc()  # idempotent
