import copy
import unittest
from bootstrap import merge_config

class MergeTests(unittest.TestCase):
    def test_scalar_model_normalization_is_narrow_and_does_not_mutate_input(self):
        from adapters.v20260914 import normalize_model_config
        current = {'model':' old-model ', 'personal':{'keep':'yes'}}
        before = copy.deepcopy(current)
        self.assertEqual(normalize_model_config(current, {'model':{'default':'declared'}}), {'model':{'default':'old-model'},'personal':{'keep':'yes'}})
        self.assertEqual(current, before)
        self.assertEqual(normalize_model_config(current, {'other':1}), before)
        with self.assertRaises(ValueError):
            merge_config({'personal':'keep'}, {'personal':{'other':1}}, set())

    def test_model_reset_preserves_personalization_and_removes_retired_key(self):
        current = {'model': {'default': 'local-choice'}, 'agent': {'system_prompt': 'Speak briefly'}, 'display': {'personality': 'my-personality'}, 'compression': {'threshold': 0.5, 'personal_note': 'keep'}}
        desired = {'model': {'default': 'cr-model'}}
        result = merge_config(current, desired, {('compression', 'threshold')})
        self.assertEqual(result['model']['default'], 'cr-model')
        self.assertEqual(result['agent']['system_prompt'], 'Speak briefly')
        self.assertEqual(result['display']['personality'], 'my-personality')
        self.assertEqual(result['compression'], {'personal_note': 'keep'})
        self.assertEqual(merge_config(result, desired, set()), result)

    def test_null_deletes_and_lists_are_leaves_without_mutating_input(self):
        current = {'a': {'b': 1, 'c': [1]}, 'x': 3}
        before = copy.deepcopy(current)
        self.assertEqual(merge_config(current, {'a': {'b': None, 'c': [2]}}, set()), {'a': {'c': [2]}, 'x': 3})
        self.assertEqual(current, before)

    def test_collision_preserves_unmanaged_descendants(self):
        with self.assertRaises(ValueError):
            merge_config({'a': {'personal': 'secret'}}, {'a': 'new'}, set())
        with self.assertRaises(ValueError):
            merge_config({'a': 'personal'}, {'a': {'b': 1}}, set())

    def test_owned_collision_can_be_replaced(self):
        self.assertEqual(merge_config({'a': {'old': 1}}, {'a': 2}, {('a', 'old')}), {'a': 2})

    def test_owned_empty_mapping_resets_reasoning_map(self):
        self.assertEqual(merge_config({'agent':{'reasoning_overrides':{'temporary':'high'}}}, {'agent':{'reasoning_overrides':{}}}, {('agent','reasoning_overrides')}), {'agent':{'reasoning_overrides':{}}})
