insert into whitelist_domains (domain, weight, category) values
  ('inta.gob.ar', 100, 'gobierno'),
  ('conicet.gov.ar', 100, 'gobierno'),
  ('argentina.gob.ar', 90, 'gobierno'),
  ('scielo.org', 90, 'ciencia'),
  ('scielo.org.ar', 90, 'ciencia'),
  ('redalyc.org', 80, 'ciencia'),
  ('fao.org', 85, 'organismo internacional'),
  ('unlp.edu.ar', 80, 'universidad'),
  ('uba.ar', 80, 'universidad'),
  ('unc.edu.ar', 80, 'universidad')
on conflict (domain) do nothing;
