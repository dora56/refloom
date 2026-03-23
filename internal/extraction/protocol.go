package extraction

// Request is the JSON request sent to the Python worker.
type Request struct {
	Command string  `json:"command"`
	Path    string  `json:"path"`
	Format  string  `json:"format"`
	Options Options `json:"options"`
}

// Options for extraction.
type Options struct {
	ChunkSize    int `json:"chunk_size"`
	ChunkOverlap int `json:"chunk_overlap"`
}

// Response is the JSON response from the Python worker.
type Response struct {
	Status   string        `json:"status"`
	Error    string        `json:"error,omitempty"`
	Details  string        `json:"details,omitempty"`
	Book     BookInfo      `json:"book"`
	Chapters []ChapterInfo `json:"chapters"`
	Chunks   []ChunkInfo   `json:"chunks"`
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
