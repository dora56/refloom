CREATE VIRTUAL TABLE IF NOT EXISTS chunk_vec_binary USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding bit[768]
);
