import { css } from '@emotion/css';
import { useMemo } from 'react';

import { GrafanaTheme2, TimeZoneInfo } from '@grafana/data';
import { t } from '@grafana/i18n';

import { useStyles2 } from '../../../themes/ThemeContext';

interface Props {
  info?: TimeZoneInfo;
}

export const TimeZoneDescription = ({ info }: Props) => {
  const styles = useStyles2(getStyles);
  const description = useDescription(info);

  if (!info) {
    return null;
  }

  return <div className={styles.description}>{description}</div>;
};

const normalizeCountryName = (name: string): string => {
  const regex = /\s|'|\(|\)|\./g;
  return name.replace(regex, '_');
};

const useDescription = (info?: TimeZoneInfo): string => {
  return useMemo(() => {
    const parts: string[] = [];

    if (!info) {
      return '';
    }

    if (info.name === 'Europe/Simferopol') {
      // See https://github.com/grafana/grafana/issues/72031
      return 'Ukraine, EEST';
    }

    if (info.countries.length > 0) {
      const country = info.countries[0];
      parts.push(t(`grafana-data.datetime.timezones.country.${normalizeCountryName(country.name)}`, country.name));
    }

    if (info.abbreviation) {
      parts.push(t(`grafana-data.datetime.timezones.timezone.${info.abbreviation}`, info.abbreviation));
    }

    return parts.join(', ');
  }, [info]);
};

const getStyles = (theme: GrafanaTheme2) => {
  return {
    description: css({
      fontWeight: 'normal',
      fontSize: theme.typography.size.sm,
      color: theme.colors.text.secondary,
      whiteSpace: 'normal',
      textOverflow: 'ellipsis',
    }),
  };
};
