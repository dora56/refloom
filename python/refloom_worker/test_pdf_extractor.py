"""Tests for PDF extraction OCR selection helpers."""

from refloom_worker.pdf_extractor import (
    _load_ocr_settings,
    _non_space_len,
    _preferred_ocr_text,
    _sample_page_numbers,
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
