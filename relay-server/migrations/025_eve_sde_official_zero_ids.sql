-- CCP's official SDE contains reserved/system records with type_id/group_id 0.
-- Keep them importable for build completeness; published market candidates still
-- require published=true, positive volume, and matching live market orders.
ALTER TABLE eve_types DROP CONSTRAINT eve_types_type_id_check;
ALTER TABLE eve_types DROP CONSTRAINT eve_types_group_id_check;
ALTER TABLE eve_types ADD CONSTRAINT eve_types_type_id_check CHECK (type_id >= 0);
ALTER TABLE eve_types ADD CONSTRAINT eve_types_group_id_check CHECK (group_id >= 0);
