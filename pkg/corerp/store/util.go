package store

import "strings"

// https://msazure.visualstudio.com/DefaultCollection/One/_git/AzureUX-ResourceStack?path=/src/common/core/Utilities/NormalizationUtility.cs&_a=contents&version=GBmaster

// NormalizeLetterOrDigitToUpper normalizes the value to only letter or digit to upper invariant.
func NormalizeLetterOrDigitToUpper(s string) string {
	if s == "" {
		return s
	}

	sb := strings.Builder{}
	for _, ch := range s {
		sb.WriteByte(byte(ch))
	}
	return strings.ToUpper(sb.String())
}

// NomalizeSubscriptionID normalizes subscription id.
func NomalizeSubscriptionID(subscriptionID string) string {
	return NormalizeLetterOrDigitToUpper(subscriptionID)
}

// NormalizeLocation normalizes location. e.g. "West US" -> "WESTUS"
func NormalizeLocation(location string) string {
	return NormalizeLocation(location)
}

/*
https://msazure.visualstudio.com/DefaultCollection/One/_git/AzureUX-AZSaaS?path=%2Fsrc%2Fcommon%2Fstorage%2Fhelpers%2Fstorageutility.cs&_a=contents&version=GBmaster

        /// <summary>
        /// The escaped storage keys.
        /// </summary>
        private static readonly string[] EscapedStorageKeys = new string[128]
        {
            ":00", ":01", ":02", ":03", ":04", ":05", ":06", ":07", ":08", ":09", ":0A", ":0B", ":0C", ":0D", ":0E", ":0F",
            ":10", ":11", ":12", ":13", ":14", ":15", ":16", ":17", ":18", ":19", ":1A", ":1B", ":1C", ":1D", ":1E", ":1F",
            ":20", ":21", ":22", ":23", ":24", ":25", ":26", ":27", ":28", ":29", ":2A", ":2B", ":2C", ":2D", ":2E", ":2F",
            "0",   "1",   "2",   "3",   "4",   "5",   "6",   "7",   "8",   "9", ":3A", ":3B", ":3C", ":3D", ":3E", ":3F",
            ":40",   "A",   "B",   "C",   "D",   "E",   "F",   "G",   "H",   "I",   "J",   "K",   "L",   "M",   "N",   "O",
            "P",   "Q",   "R",   "S",   "T",   "U",   "V",   "W",   "X",   "Y",   "Z", ":5B", ":5C", ":5D", ":5E", ":5F",
            ":60",   "a",   "b",   "c",   "d",   "e",   "f",   "g",   "h",   "i",   "j",   "k",   "l",   "m",   "n",   "o",
            "p",   "q",   "r",   "s",   "t",   "u",   "v",   "w",   "x",   "y",   "z", ":7B", ":7C", ":7D", ":7E", ":7F",
        };

		      /// <summary>
        /// Escapes the storage key.
        /// </summary>
        /// <param name="storageKey">The storage key.</param>
        public static string EscapeStorageKey(string storageKey)
        {
            var sb = new StringBuilder(storageKey.Length);
            for (var index = 0; index < storageKey.Length; ++index)
            {
                var c = storageKey[index];
                if (c < 128)
                {
                    sb.Append(StorageUtility.EscapedStorageKeys[c]);
                }
                else if (char.IsLetterOrDigit(c))
                {
                    sb.Append(c);
                }
                else if (c < 0x100)
                {
                    sb.Append(':');
                    sb.Append(((int)c).ToString("X2", CultureInfo.InvariantCulture));
                }
                else
                {
                    sb.Append(':');
                    sb.Append(':');
                    sb.Append(((int)c).ToString("X4", CultureInfo.InvariantCulture));
                }
            }

            return sb.ToString();

			/// <summary>
        /// Combines the storage keys.
        /// </summary>
        /// <param name="keys">The storage keys.</param>
        public static string CombineStorageKeys(params string[] keys)
        {
            foreach (var key in keys)
            {
                if (key.Contains('-', StringComparison.InvariantCulture))
                {
                    throw new ArgumentException(string.Format(CultureInfo.InvariantCulture, "The storage key '{0}' is not properly encoded. Use CloudTableExtensions.EscapeStorageKey for encoding.", key), nameof(keys));
                }
            }

            return string.Join("-", keys);
        }

        /// <summary>
        /// Combines the storage keys.
        /// </summary>
        /// <param name="keys0">The keys0.</param>
        /// <param name="keys1">The keys1.</param>
        public static string CombineStorageKeys(string keys0, string keys1)
        {
            if (keys0.Contains('-', StringComparison.InvariantCulture))
            {
                throw new ArgumentException(string.Format(CultureInfo.InvariantCulture, "The storage key '{0}' is not properly encoded. Use CloudTableExtensions.EscapeStorageKey for encoding.", keys0), nameof(keys0));
            }

            if (keys1.Contains('-', StringComparison.InvariantCulture))
            {
                throw new ArgumentException(string.Format(CultureInfo.InvariantCulture, "The storage key '{0}' is not properly encoded. Use CloudTableExtensions.EscapeStorageKey for encoding.", keys1), nameof(keys1));
            }

            return string.Concat(keys0, "-", keys1);
        }
*/
