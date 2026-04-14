#![no_std]
use soroban_sdk::{contract, contractimpl, contracttype, symbol_short, Env, String, Symbol, Vec};

// Struktur data yang akan menyimpan notes
#[contracttype]
#[derive(Clone, Debug)]
pub struct Note {

}

// Storage key untuk data notes
// const NOTE_DATA: Symbol = symbol_short!("NOTE_DATA");

#[contract]
pub struct NotesContract;

#[contractimpl]
impl NotesContract {
    // Fungsi untuk mendapatkan semua notes
    pub fn get_notes(env: Env) -> Vec<Note> {
        // 1. ambil data notes dari storage
        
        return [];
    }

    // Fungsi untuk membuat note baru
    pub fn create_note(env: Env, title: String, content: String) -> String {
        // 1. ambil data notes dari storage
        
        // 2. Buat object note baru
        
        // 3. tambahkan note baru ke notes lama
        
        // 4. simpan notes ke storage
        
        return String::from_str(&env, "Notes berhasil ditambahkan");
    }

    // Fungsi untuk menghapus notes berdasarkan id
    pub fn delete_note(env: Env, id: u64) -> String {
        // 1. ambil data notes dari storage 

        // 2. cari index note yang akan dihapus menggunakan perulangan

        return String::from_str(&env, "Notes tidak ditemukan")
    }
}

mod test;













/* --- CONTOH SCRIPT ---

pub fn get_notes(env: Env) -> Vec<Note> {
    // 1. ambil data notes dari storage
    return env.storage().instance().get(&NOTE_DATA).unwrap_or(Vec::new(&env));
}

// Fungsi untuk membuat note baru
pub fn create_note(env: Env, title: String, content: String) -> String {
    // 1. ambil data notes dari storage
    let mut notes: Vec<Note> = env.storage().instance().get(&NOTE_DATA).unwrap_or(Vec::new(&env));
    
    // 2. Buat object note baru
    let note = Note {
        id: env.prng().gen::<u64>(),
        title: title,
        content: content,
    };
    
    // 3. tambahkan note baru ke notes lama
    notes.push_back(note);
    
    // 4. simpan notes ke storage
    env.storage().instance().set(&NOTE_DATA, &notes);
    
    return String::from_str(&env, "Notes berhasil ditambahkan");
}

// Fungsi untuk menghapus notes berdasarkan id
pub fn delete_note(env: Env, id: u64) -> String {
    // 1. ambil data notes dari storage 
    let mut notes: Vec<Note> = env.storage().instance().get(&NOTE_DATA).unwrap_or(Vec::new(&env));

    // 2. cari index note yang akan dihapus menggunakan perulangan
    for i in 0..notes.len() {
        if notes.get(i).unwrap().id == id {
            notes.remove(i);

            env.storage().instance().set(&NOTE_DATA, &notes);
            return String::from_str(&env, "Berhasil hapus notes");
        }
    }

    return String::from_str(&env, "Notes tidak ditemukan")
}


*/