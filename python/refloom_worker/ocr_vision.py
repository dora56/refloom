"""OCR using Apple Vision Framework (macOS only).

Uses VNRecognizeTextRequest to extract text from image data.
Requires pyobjc-framework-Vision and pyobjc-framework-Quartz.
"""

import sys


def is_available() -> bool:
    """Check if Apple Vision Framework is available."""
    if sys.platform != "darwin":
        return False
    try:
        import Quartz  # noqa: F401
        import Vision  # noqa: F401
        return True
    except ImportError:
        return False


def recognize_text(
    image_data: bytes,
    languages: list[str] | None = None,
    recognition_level: int = 0,
) -> list[str]:
    """Extract text from image data using VNRecognizeTextRequest.

    Args:
        image_data: Raw image bytes (PNG, JPEG, etc.)
        languages: Recognition languages (default: ["ja", "en"])
        recognition_level: 0 = accurate, 1 = fast

    Returns:
        List of recognized text strings, one per text observation.
    """
    if languages is None:
        languages = ["ja", "en"]

    import Quartz
    import Vision
    from Foundation import NSData

    ns_data = NSData.dataWithBytes_length_(image_data, len(image_data))
    ci_image = Quartz.CIImage.imageWithData_(ns_data)
    if ci_image is None:
        return []

    handler = Vision.VNImageRequestHandler.alloc().initWithCIImage_options_(
        ci_image, None
    )

    results: list[str] = []

    def completion(request, error):
        if error:
            return
        observations = request.results()
        if not observations:
            return
        for obs in observations:
            candidates = obs.topCandidates_(1)
            if candidates:
                results.append(candidates[0].string())

    request = Vision.VNRecognizeTextRequest.alloc().initWithCompletionHandler_(
        completion
    )
    request.setRecognitionLevel_(recognition_level)
    request.setRecognitionLanguages_(languages)

    success, error = handler.performRequests_error_([request], None)
    if not success:
        return []

    return results
