package devices

import "strings"

// OUI is a curated organizationally unique identifier map. It is intentionally
// a small, offline set of common home-networking vendors; unknown OUIs resolve
// to "Unknown" rather than fabricating an identity.
type OUIMap map[string]string

// OUI is the global vendor lookup table.
var OUI = OUIMap{
	// Raspberry Pi / arm SBCs
	"DCA632": "Raspberry Pi", "B827EB": "Raspberry Pi", "E45F01": "Raspberry Pi",
	"28CDC1": "Raspberry Pi", "2CF0A2": "Raspberry Pi", "B8A43B": "Raspberry Pi",
	"DC9E1B": "Rockchip", "C85978": "Espressif", "24A1C7": "Espressif", "24B2DE": "Espressif",
	"18FE34": "Espressif", "5CCF7F": "Espressif", "30AEA4": "Espressif",
	// Networking
	"00E04C": "Realtek", "EC8EB5": "Realtek", "D8199A": "Realtek", "F4C715": "Realtek",
	"001A2B": "TP-Link", "50C7BF": "TP-Link", "04D9F5": "TP-Link", "14CC20": "TP-Link",
	"A0F4B6": "TP-Link", "C0A55B": "TP-Link", "F8D111": "TP-Link",
	"00A57E": "Xiaomi", "F064E3": "Xiaomi", "78FFCA": "Xiaomi", "0C6EB3": "Xiaomi",
	"882593": "Xiaomi", "18C58A": "Xiaomi",
	"A8DA0C": "Huawei", "F45C89": "Huawei", "7C4F05": "Huawei", "00E0FC": "Huawei",
	"047A66": "Asus", "90B11C": "Asus", "8C633B": "Asus", "048D38": "Asus",
	"3CE5A6": "Zyxel", "286C07": "Netgear", "A046C1": "Netgear", "C04A00": "Netgear",
	"F0265D": "Netgear", "C025E9": "Netgear",
	"00B0C2": "D-Link", "C8074A": "D-Link", "1CAAB7": "D-Link",
	"1A5B28": "Ubiquiti", "44D9E7": "Ubiquiti", "DC9FDB": "Ubiquiti", "68D719": "Ubiquiti",
	"04F021": "MikroTik", "48DF37": "MikroTik", "640666": "MikroTik",
	// Phones / tablets
	"001DEA": "Apple", "A4780D": "Apple", "ACD1B8": "Apple", "F04DA2": "Apple",
	"3CD97E": "Apple", "D0E140": "Apple", "98E7F5": "Apple", "3497F6": "Apple",
	"506F9A": "Apple", "8C7B9D": "Apple", "F4F15A": "Apple", "C8E0EB": "Apple",
	"A0D311": "Apple", "808698": "Apple", "6C3E6D": "Apple",
	"9417C4": "Google", "5404A6": "Google", "00184D": "Google", "6C3836": "Google",
	"28C6FA": "Google", "4CB16C": "Google", "50D419": "Google", "789F5F": "Google",
	"ACFB55": "Google", "B0B2DC": "Google", "D89695": "Google", "18C5CE": "Google",
	"F0DEF1": "Google", "A0BF57": "Google", "805A04": "Google", "3C8AB0": "Google",
	"04AF00": "OnePlus", "08970F": "OnePlus",
	"00153F": "Samsung", "00253E": "Samsung", "20A987": "Samsung", "34D2C4": "Samsung",
	"5CC1ED": "Samsung", "B4E9B0": "Samsung", "D8B04E": "Samsung", "EC3171": "Samsung",
	"A4580F": "Samsung", "4CE1BB": "Samsung", "E81F2B": "Samsung", "0025FB": "Samsung",
	"8C0B83": "HTC", "F0D29D": "HTC",
	"0403D4": "Sony", "40D248": "Sony", "AC5B98": "Sony", "C8A230": "Sony",
	"0CE0E4": "LG", "705494": "LG", "2CAE2A": "LG",
	"68864B": "Motorola", "0009C9": "Nokia", "E8E1B8": "Nokia", "E848D0": "Nokia",
	"38E6AD": "Lenovo", "200EB8": "Lenovo", "8C11CB": "Lenovo", "9CD332": "Lenovo",
	"7C1158": "Huawei",
	// Laptops / PCs
	"3C970E": "Intel", "00154D": "Intel", "00241F": "Intel", "0016EA": "Intel",
	"0025AE": "Intel", "88A5BD": "Intel", "F82FA8": "Intel", "50E549": "Intel",
	"5CF5DA": "Intel", "E84040": "Intel", "B09AE2": "Intel", "D45265": "Intel",
	"F8BC12": "Dell", "E46DDA": "Dell", "18B79E": "Dell", "A47D5C": "Dell",
	"F057E4": "Dell", "78ACC0": "Dell",
	"00E09B": "Apple", "001A4B": "Apple", "E0ACCB": "Apple", "38C986": "Apple",
	"2C4C43": "Apple", "F0F6C1": "Apple",
	"000011": "Toshiba", "001B44": "Toshiba", "54FAB3": "Toshiba",
	"0026AB": "Sony", "5C33B8": "Sony",
	"004008": "ASUS", "02B19C": "MSI", "001D72": "MSI", "D48C63": "MSI",
	"1045CC": "MSI", "60B185": "MSI",
	"00037B": "HP", "28D53F": "HP", "3C6AA7": "HP", "60B98B": "HP",
	"AC88FD": "HP", "D065B4": "HP", "E46AB1": "HP",
	"048C03": "ZTE", "28D93A": "ZTE", "DCD2FC": "ZTE",
	// Storage / NAS
	"001132": "Synology", "A000BB": "Synology", "001477": "QNAP",
	"FF4641": "Western Digital", "00601D": "Western Digital", "00144C": "Seagate",
	"002538": "Seagate",
	// Misc common
	"0011A6": "Qualcomm", "9C51EB": "Qualcomm", "B0487A": "Qualcomm", "F8D0AC": "Qualcomm",
	"00248C": "Broadcom", "0021FB": "Broadcom", "FC6BD0": "Broadcom",
	"000217": "Belkin", "08F9E0": "Belkin", "F0F5AE": "Belkin",
	"1C7C11": "Acer", "08606E": "Acer",
	"0017F6": "Huawei", "181DC9": "Huawei",
	"5C2E59": "Amazon", "AC63BE": "Amazon", "38F7BD": "Amazon",
	"F4B8E5": "Roku", "C02D1F": "Roku", "EC44A7": "Vizio",
}

// Lookup returns the vendor name for a MAC address, or "Unknown".
func (o OUIMap) Lookup(mac string) string {
	hex := strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
	if len(hex) < 6 {
		return "Unknown"
	}
	if v, ok := o[hex[:6]]; ok {
		return v
	}
	return "Unknown"
}
