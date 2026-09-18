import json
from pathlib import Path
import sqlite3
from contextlib import closing
import tempfile
import unittest
from adapters.v20260914 import reset_model_overrides

class SessionsTests(unittest.TestCase):
    def test_primary_and_mirror_preserve_identity_and_history(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            (home / 'sessions').mkdir()
            entry = {'session_id': 'original', 'model_override': {'model': 'temporary'}, 'personal': 'keep'}
            (home / 'sessions/sessions.json').write_text(json.dumps({'route': entry}))
            with closing(sqlite3.connect(home / 'state.db')) as db, db:
                db.execute('CREATE TABLE gateway_routing(scope TEXT, session_key TEXT, entry_json TEXT)')
                db.execute('INSERT INTO gateway_routing VALUES (?,?,?)', (str((home / 'sessions').resolve()), 'route', json.dumps(entry)))
                db.execute('CREATE TABLE history(content TEXT)')
                db.execute("INSERT INTO history VALUES ('keep')")
            reset_model_overrides(home)
            reset_model_overrides(home)
            with closing(sqlite3.connect(home / 'state.db')) as db, db:
                data = json.loads(db.execute('SELECT entry_json FROM gateway_routing').fetchone()[0])
                self.assertEqual(data['session_id'], 'original')
                self.assertIsNone(data.get('model_override'))
                self.assertEqual(db.execute('SELECT content FROM history').fetchone()[0], 'keep')
            self.assertIsNone(json.loads((home/'sessions/sessions.json').read_text())['route'].get('model_override'))

    def test_corrupt_database_fails_without_deleting_data(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            (home/'state.db').write_bytes(b'original broken db')
            with self.assertRaises(RuntimeError):
                reset_model_overrides(home)
            self.assertEqual((home/'state.db').read_bytes(), b'original broken db')
