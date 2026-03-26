package extraction

// ProbeRequest asks the Python worker for document metadata and extraction guidance.
type ProbeRequest struct {
	Command string `json:"command"`
	Path    string `json:"path"`
	Format  string `json:"format"`
}

// ProbeResponse contains metadata needed to orchestrate staged extraction.
type ProbeResponse struct {
	Status                    string        `json:"status"`
	Error                     string        `json:"error,omitempty"`
	Details                   string        `json:"details,omitempty"`
	Book                      BookInfo      `json:"book"`
	Chapters                  []ChapterInfo `json:"chapters"`
	ExtractionMode            string        `json:"extraction_mode,omitempty"`
	RecommendedBatchSize      int           `json:"recommended_batch_size,omitempty"`
	OCRCandidatePagesEstimate int           `json:"ocr_candidate_pages_estimate,omitempty"`
}

// ExtractPagesRequest extracts a bounded page range and writes JSONL to disk.
type ExtractPagesRequest struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	Format     string `json:"format"`
	PageStart  int    `json:"page_start"`
	PageEnd    int    `json:"page_end"`
	OCRPolicy  string `json:"ocr_policy,omitempty"`
	FileHash   string `json:"file_hash,omitempty"`
	OutputPath string `json:"output_path"`
}

// ExtractPagesResponse reports batch-level extraction results.
type ExtractPagesResponse struct {
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	Details      string `json:"details,omitempty"`
	PagesWritten int    `json:"pages_written"`
	Stats        Stats  `json:"stats"`
	BatchMS      int64  `json:"batch_ms"`
}

// ChunkRequest converts persisted pages into persisted chunks.
type ChunkRequest struct {
	Command      string  `json:"command"`
	Format       string  `json:"format"`
	PagesPath    string  `json:"pages_path"`
	ChaptersPath string  `json:"chapters_path"`
	OutputPath   string  `json:"output_path"`
	Options      Options `json:"options"`
}

// ChunkResponse reports the chunking outcome.
type ChunkResponse struct {
	Status        string `json:"status"`
	Quality       string `json:"quality,omitempty"`
	Error         string `json:"error,omitempty"`
	Details       string `json:"details,omitempty"`
	ChunksWritten int    `json:"chunks_written"`
	ChunkMS       int64  `json:"chunk_ms"`
}

// Options for chunking.
type Options struct {
	ChunkSize    int `json:"chunk_size"`
	ChunkOverlap int `json:"chunk_overlap"`
}

// Stats contains extraction-time counters returned by the worker.
type Stats struct {
	OCRPages      int   `json:"ocr_pages"`
	OCRRetries    int   `json:"ocr_retries"`
	OCRMS         int64 `json:"ocr_ms"`
	OCRFastPages  int   `json:"ocr_fast_pages"`
	OCRRetryPages int   `json:"ocr_retry_pages"`
	OCRFastMS     int64 `json:"ocr_fast_ms,omitempty"`
	OCRRetryMS    int64 `json:"ocr_retry_ms,omitempty"`
}

// BookInfo contains extracted book metadata.
type BookInfo struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Format    string `json:"format"`
	PageCount int    `json:"page_count"`
}

// ChapterInfo contains extracted chapter metadata.
type ChapterInfo struct {
	Title     string `json:"title"`
	Order     int    `json:"order"`
	PageStart *int   `json:"page_start"`
	PageEnd   *int   `json:"page_end"`
}

// PageInfo contains one extracted page persisted as JSONL.
type PageInfo struct {
	PageNum int    `json:"page_num"`
	Text    string `json:"text"`
}

// ChunkInfo contains extracted chunk data.
type ChunkInfo struct {
	ChapterOrder int    `json:"chapter_order"`
	Heading      string `json:"heading"`
	Body         string `json:"body"`
	CharCount    int    `json:"char_count"`
	PageStart    *int   `json:"page_start"`
	PageEnd      *int   `json:"page_end"`
	ChunkOrder   int    `json:"chunk_order"`
}
