"""Tests for Vision Framework OCR wrapper."""

import sys

import pytest

from refloom_worker.ocr_vision import is_available


@pytest.mark.skipif(sys.platform != "darwin", reason="macOS only")
class TestVisionOCR:
    def test_is_available(self):
        """Vision Framework should be available on macOS."""
        assert is_available() is True

    def test_recognize_empty_image(self):
        """Empty/invalid image data should return empty list."""
        from refloom_worker.ocr_vision import recognize_text
        result = recognize_text(b"")
        assert result == []

    def test_recognize_invalid_data(self):
        """Random bytes should return empty list."""
        from refloom_worker.ocr_vision import recognize_text
        result = recognize_text(b"\x00\x01\x02\x03")
        assert result == []
